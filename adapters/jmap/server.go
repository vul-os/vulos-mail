// Package jmap is the JMAP (RFC 8620/8621) protocol edge — the modern native
// protocol. It hand-rolls the HTTP/JSON surface over account.Runtime, mapping
// the domain model directly: labels are Mailboxes, flags+label-membership are
// Email keywords/mailboxIds. This is the protocol the webmail speaks.
//
// Surface: Session, Mailbox/get, Email/query (inMailbox filter), Email/get,
// Email/set (create draft / keywords + mailboxIds patch / destroy), Identity/get,
// and EmailSubmission/set (send, with onSuccessUpdateEmail to move Drafts->Sent)
// with within-request "#creationId" back-references.
package jmap

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/authlimit"
	"github.com/vul-os/vulos-mail/internal/compose"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/model"
)

// Backend resolves HTTP Basic credentials to an account runtime + account id.
type Backend struct {
	Auth func(username, password string) (*account.Runtime, error)
	// Submit, if set, sends a raw message on behalf of account (for
	// EmailSubmission/set). Without it, submission fails with forbidden.
	Submit func(ctx context.Context, account string, raw []byte) error
	// Limiter, if set, throttles brute-force Basic-auth attempts per client IP
	// and per account (failed attempts lock out; a success resets).
	Limiter *authlimit.Limiter
}

// Handler returns the JMAP HTTP handler (/jmap/session and /jmap/api).
func (b *Backend) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/jmap/session", b.withAuth(b.session))
	mux.HandleFunc("/jmap/api", b.withAuth(b.api))
	return mux
}

type authedFunc func(w http.ResponseWriter, r *http.Request, rt *account.Runtime, accountID string)

func (b *Backend) withAuth(fn authedFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok {
			w.Header().Set("WWW-Authenticate", `Basic realm="jmap"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ip := remoteIP(r)
		if b.Limiter != nil && b.Limiter.AnyLocked(ip, user) {
			http.Error(w, "too many failed attempts; try again later", http.StatusTooManyRequests)
			return
		}
		rt, err := b.Auth(user, pass)
		if err != nil {
			if b.Limiter != nil {
				b.Limiter.Fail(ip, user)
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if b.Limiter != nil {
			b.Limiter.Success(ip, user)
		}
		fn(w, r, rt, user)
	}
}

// remoteIP returns the connecting peer's IP. It deliberately uses RemoteAddr
// (not X-Forwarded-For) so the brute-force key can't be spoofed by a client
// header; a fronting proxy should rewrite RemoteAddr or terminate auth itself.
func remoteIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- Session resource (RFC 8620 §2) ---

func (b *Backend) session(w http.ResponseWriter, _ *http.Request, _ *account.Runtime, accountID string) {
	writeJSON(w, map[string]any{
		"capabilities": map[string]any{
			"urn:ietf:params:jmap:core":       map[string]any{"maxObjectsInGet": 1000, "maxObjectsInSet": 1000},
			"urn:ietf:params:jmap:mail":       map[string]any{},
			"urn:ietf:params:jmap:submission": map[string]any{},
		},
		"accounts": map[string]any{
			accountID: map[string]any{
				"name":       accountID,
				"isPersonal": true,
				"isReadOnly": false,
				"accountCapabilities": map[string]any{
					"urn:ietf:params:jmap:mail":       map[string]any{},
					"urn:ietf:params:jmap:submission": map[string]any{},
				},
			},
		},
		"primaryAccounts": map[string]any{
			"urn:ietf:params:jmap:mail": accountID,
		},
		"apiUrl":         "/jmap/api",
		"downloadUrl":    "/jmap/download/{accountId}/{blobId}",
		"uploadUrl":      "/jmap/upload/{accountId}",
		"eventSourceUrl": "/jmap/eventsource",
		"state":          "0",
	})
}

// --- API request/response envelope (RFC 8620 §3.3) ---

type apiRequest struct {
	Using       []string          `json:"using"`
	MethodCalls []json.RawMessage `json:"methodCalls"` // each: [name, args, callId]
}

func (b *Backend) api(w http.ResponseWriter, r *http.Request, rt *account.Runtime, accountID string) {
	var req apiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	responses := make([]json.RawMessage, 0, len(req.MethodCalls))
	// creationIds maps a "#creationId" (within this request) to the real id it
	// resolved to, so later method calls can back-reference created objects
	// (e.g. EmailSubmission referencing an Email created earlier in the request).
	creationIds := map[string]string{}
	for _, mc := range req.MethodCalls {
		var triple []json.RawMessage
		if err := json.Unmarshal(mc, &triple); err != nil || len(triple) != 3 {
			continue
		}
		var name, callID string
		_ = json.Unmarshal(triple[0], &name)
		_ = json.Unmarshal(triple[2], &callID)
		args := triple[1]

		result := b.dispatch(rt, accountID, name, args, creationIds)
		responses = append(responses, encodeResponse(result.name, result.body, callID))
	}
	writeJSON(w, map[string]any{
		"methodResponses": rawArray(responses),
		"sessionState":    "0",
	})
}

type methodResult struct {
	name string
	body any
}

func encodeResponse(name string, body any, callID string) json.RawMessage {
	arr := []any{name, body, callID}
	b, _ := json.Marshal(arr)
	return b
}

// rawArray makes []json.RawMessage marshal as a JSON array of arrays.
func rawArray(items []json.RawMessage) json.RawMessage {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, it := range items {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.Write(it)
	}
	sb.WriteByte(']')
	return json.RawMessage(sb.String())
}

func methodError(typ string) methodResult {
	return methodResult{name: "error", body: map[string]any{"type": typ}}
}

func (b *Backend) dispatch(rt *account.Runtime, accountID, name string, args json.RawMessage, creationIds map[string]string) methodResult {
	switch name {
	case "Mailbox/get":
		return mailboxGet(rt, accountID)
	case "Email/query":
		return emailQuery(rt, accountID, args)
	case "Email/get":
		return emailGet(rt, accountID, args)
	case "Email/set":
		return emailSet(rt, accountID, args, creationIds)
	case "Identity/get":
		return identityGet(accountID)
	case "EmailSubmission/set":
		return b.emailSubmissionSet(rt, accountID, args, creationIds)
	default:
		return methodError("unknownMethod")
	}
}

// resolveRef resolves a JMAP id that may be a "#creationId" back-reference.
func resolveRef(id string, creationIds map[string]string) string {
	if strings.HasPrefix(id, "#") {
		if real, ok := creationIds[strings.TrimPrefix(id, "#")]; ok {
			return real
		}
	}
	return id
}

// Identity/get (RFC 8621 §6): the account's single send-as identity.
func identityGet(accountID string) methodResult {
	return methodResult{name: "Identity/get", body: map[string]any{
		"accountId": accountID, "state": "0", "notFound": []string{},
		"list": []any{map[string]any{
			"id": "i0", "name": accountID, "email": accountID,
			"replyTo": nil, "bcc": nil, "textSignature": "", "htmlSignature": "", "mayDelete": false,
		}},
	}}
}

// --- Mailbox/get ---

func mailboxGet(rt *account.Runtime, accountID string) methodResult {
	var list []any
	for _, l := range rt.Labels() {
		msgs := rt.MessagesWithLabel(l.ID)
		var unread int
		for _, m := range msgs {
			if !m.Flags[model.FlagSeen] {
				unread++
			}
		}
		list = append(list, map[string]any{
			"id":           string(l.ID),
			"name":         l.Name,
			"role":         mailboxRole(l.ID),
			"totalEmails":  len(msgs),
			"unreadEmails": unread,
			"myRights":     map[string]bool{"mayReadItems": true, "mayAddItems": true, "maySetKeywords": true},
		})
	}
	return methodResult{name: "Mailbox/get", body: map[string]any{
		"accountId": accountID, "state": "0", "list": list, "notFound": []string{},
	}}
}

func mailboxRole(id model.LabelID) any {
	switch id {
	case model.LabelInbox:
		return "inbox"
	case model.LabelArchive:
		return "archive"
	case model.LabelSent:
		return "sent"
	case model.LabelDrafts:
		return "drafts"
	case model.LabelTrash:
		return "trash"
	case model.LabelSpam:
		return "junk"
	}
	return nil
}

// --- Email/query ---

func emailQuery(rt *account.Runtime, accountID string, args json.RawMessage) methodResult {
	var a struct {
		Filter struct {
			InMailbox model.LabelID `json:"inMailbox"`
		} `json:"filter"`
	}
	_ = json.Unmarshal(args, &a)

	var msgs []*model.Message
	if a.Filter.InMailbox != "" {
		msgs = rt.MessagesWithLabel(a.Filter.InMailbox)
	} else {
		msgs = rt.AllMail()
	}
	// Newest first.
	ids := make([]string, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		ids = append(ids, string(msgs[i].ID))
	}
	return methodResult{name: "Email/query", body: map[string]any{
		"accountId": accountID, "queryState": "0", "canCalculateChanges": false,
		"position": 0, "total": len(ids), "ids": ids,
	}}
}

// --- Email/get ---

func emailGet(rt *account.Runtime, accountID string, args json.RawMessage) methodResult {
	var a struct {
		IDs        []string `json:"ids"`
		Properties []string `json:"properties"`
	}
	_ = json.Unmarshal(args, &a)

	// Body extraction is lazy: only when the client asks for preview/bodyValues
	// (it requires fetching + parsing the blob).
	wantBody := len(a.Properties) == 0
	for _, p := range a.Properties {
		if p == "preview" || p == "bodyValues" || p == "textBody" || p == "attachments" {
			wantBody = true
		}
	}

	var list []any
	var notFound []string
	for _, id := range a.IDs {
		m, ok := rt.Message(model.ID(id))
		if !ok {
			notFound = append(notFound, id)
			continue
		}
		obj := emailObject(m)
		if wantBody {
			addBody(rt, m, obj)
		}
		list = append(list, obj)
	}
	return methodResult{name: "Email/get", body: map[string]any{
		"accountId": accountID, "state": "0", "list": list, "notFound": notFound,
	}}
}

func emailObject(m *model.Message) map[string]any {
	return map[string]any{
		"id":         string(m.ID),
		"threadId":   string(m.ThreadID),
		"mailboxIds": labelMap(m),
		"keywords":   keywordMap(m),
		"size":       m.Size,
		"receivedAt": m.Envelope.Date.UTC().Format("2006-01-02T15:04:05Z"),
		"subject":    m.Envelope.Subject,
		"from":       fromObjs(m.Envelope),
		"to":         addrObjs(m.Envelope.To),
		"cc":         addrObjs(m.Envelope.Cc),
		"messageId":  []string{m.Envelope.MessageIDHeader},
	}
}

// addBody fetches the raw blob, extracts inline text, and adds JMAP preview +
// bodyValues/textBody to the object.
func addBody(rt *account.Runtime, m *model.Message, obj map[string]any) {
	raw, err := rt.Body(context.Background(), m.BlobRef)
	if err != nil {
		return
	}
	text, err := mime.ExtractText(raw)
	if err != nil {
		return
	}
	text = strings.TrimSpace(text)
	preview := text
	if len(preview) > 240 {
		preview = preview[:240] + "…"
	}
	obj["preview"] = collapseWS(preview)
	obj["bodyValues"] = map[string]any{"1": map[string]any{"value": text, "isTruncated": false}}
	obj["textBody"] = []any{map[string]any{"partId": "1", "type": "text/plain"}}
	if atts := mime.Attachments(raw); len(atts) > 0 {
		arr := make([]any, 0, len(atts))
		for i, a := range atts {
			arr = append(arr, map[string]any{"partId": i, "name": a.Name, "type": a.Type, "size": len(a.Data)})
		}
		obj["attachments"] = arr
	}
}

func collapseWS(s string) string { return strings.Join(strings.Fields(s), " ") }

// fromObjs builds the From list, attaching the display name to the first address.
func fromObjs(env model.Envelope) []map[string]any {
	out := make([]map[string]any, 0, len(env.From))
	for i, a := range env.From {
		o := map[string]any{"email": a}
		if i == 0 && env.FromName != "" {
			o["name"] = env.FromName
		}
		out = append(out, o)
	}
	return out
}

func addrObjs(addrs []string) []map[string]any {
	out := make([]map[string]any, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, map[string]any{"email": a})
	}
	return out
}

func labelMap(m *model.Message) map[string]bool {
	out := map[string]bool{}
	for l := range m.Labels {
		out[string(l)] = true
	}
	return out
}

func keywordMap(m *model.Message) map[string]bool {
	out := map[string]bool{}
	for f := range m.Flags {
		if kw := keywordForFlag(f); kw != "" {
			out[kw] = true
		}
	}
	return out
}

// --- Email/set (keywords + mailboxIds patch) ---

func emailSet(rt *account.Runtime, accountID string, args json.RawMessage, creationIds map[string]string) methodResult {
	var a struct {
		Create  map[string]json.RawMessage            `json:"create"`
		Update  map[string]map[string]json.RawMessage `json:"update"`
		Destroy []string                              `json:"destroy"`
	}
	_ = json.Unmarshal(args, &a)

	ctx := context.Background()
	created := map[string]any{}
	notCreated := map[string]any{}
	updated := map[string]any{}
	notUpdated := map[string]any{}
	destroyed := []string{}

	// create (e.g. compose a draft) — deterministic order for testability.
	ckeys := make([]string, 0, len(a.Create))
	for k := range a.Create {
		ckeys = append(ckeys, k)
	}
	sort.Strings(ckeys)
	for _, key := range ckeys {
		id, err := createEmail(ctx, rt, a.Create[key])
		if err != nil {
			notCreated[key] = map[string]any{"type": "invalidProperties", "description": err.Error()}
			continue
		}
		creationIds[key] = string(id)
		created[key] = map[string]any{"id": string(id), "blobId": string(id), "threadId": threadOf(rt, id), "size": sizeOf(rt, id)}
	}

	ids := make([]string, 0, len(a.Update))
	for id := range a.Update {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, raw := range ids {
		id := resolveRef(raw, creationIds)
		patch := a.Update[raw]
		if _, ok := rt.Message(model.ID(id)); !ok {
			notUpdated[raw] = map[string]any{"type": "notFound"}
			continue
		}
		if err := applyPatch(ctx, rt, model.ID(id), patch); err != nil {
			notUpdated[raw] = map[string]any{"type": "invalidPatch"}
			continue
		}
		updated[raw] = nil
	}

	for _, raw := range a.Destroy {
		id := resolveRef(raw, creationIds)
		if err := rt.Expunge(ctx, model.ID(id)); err == nil {
			destroyed = append(destroyed, raw)
		}
	}

	return methodResult{name: "Email/set", body: map[string]any{
		"accountId": accountID, "oldState": "0", "newState": "0",
		"created": created, "notCreated": notCreated,
		"updated": updated, "notUpdated": notUpdated, "destroyed": destroyed,
	}}
}

func threadOf(rt *account.Runtime, id model.ID) string {
	if m, ok := rt.Message(id); ok {
		return string(m.ThreadID)
	}
	return ""
}

func sizeOf(rt *account.Runtime, id model.ID) int64 {
	if m, ok := rt.Message(id); ok {
		return m.Size
	}
	return 0
}

// createEmail builds a MIME message from JMAP Email properties and ingests it
// (typically a draft). It supports from/to/cc/subject + text/html bodyValues.
func createEmail(ctx context.Context, rt *account.Runtime, raw json.RawMessage) (model.ID, error) {
	var e struct {
		MailboxIds map[string]bool                   `json:"mailboxIds"`
		Keywords   map[string]bool                   `json:"keywords"`
		From       []map[string]string               `json:"from"`
		To         []map[string]string               `json:"to"`
		Cc         []map[string]string               `json:"cc"`
		Subject    string                            `json:"subject"`
		TextBody   []map[string]string               `json:"textBody"`
		HTMLBody   []map[string]string               `json:"htmlBody"`
		BodyValues map[string]struct{ Value string } `json:"bodyValues"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		return "", err
	}
	addr := func(list []map[string]string) []string {
		out := make([]string, 0, len(list))
		for _, a := range list {
			if a["email"] != "" {
				out = append(out, a["email"])
			}
		}
		return out
	}
	bodyFor := func(parts []map[string]string) string {
		var sb strings.Builder
		for _, p := range parts {
			if v, ok := e.BodyValues[p["partId"]]; ok {
				sb.WriteString(v.Value)
			}
		}
		return sb.String()
	}
	var from string
	if len(e.From) > 0 {
		from = e.From[0]["email"]
	}
	msg := compose.Message{
		From: from, To: addr(e.To), Cc: addr(e.Cc), Subject: e.Subject,
		Text: bodyFor(e.TextBody), HTML: bodyFor(e.HTMLBody),
	}
	body, err := compose.Build(msg)
	if err != nil {
		return "", err
	}
	labels := make([]model.LabelID, 0, len(e.MailboxIds))
	for l, on := range e.MailboxIds {
		if on {
			labels = append(labels, model.LabelID(l))
		}
	}
	if len(labels) == 0 {
		labels = []model.LabelID{model.LabelDrafts}
	}
	var flags []model.Flag
	for kw, on := range e.Keywords {
		if on {
			if f := flagForKeyword(kw); f != "" {
				flags = append(flags, f)
			}
		}
	}
	return rt.Ingest(ctx, body, labels, flags)
}

// EmailSubmission/set: send an existing (draft) email, with optional
// onSuccessUpdateEmail to relabel it (e.g. Drafts -> Sent).
func (b *Backend) emailSubmissionSet(rt *account.Runtime, accountID string, args json.RawMessage, creationIds map[string]string) methodResult {
	if b.Submit == nil {
		return methodError("forbidden")
	}
	var a struct {
		Create map[string]struct {
			EmailID    string `json:"emailId"`
			IdentityID string `json:"identityId"`
		} `json:"create"`
		OnSuccessUpdateEmail map[string]map[string]json.RawMessage `json:"onSuccessUpdateEmail"`
	}
	_ = json.Unmarshal(args, &a)

	ctx := context.Background()
	created := map[string]any{}
	notCreated := map[string]any{}

	keys := make([]string, 0, len(a.Create))
	for k := range a.Create {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sub := a.Create[key]
		emailID := resolveRef(sub.EmailID, creationIds)
		m, ok := rt.Message(model.ID(emailID))
		if !ok {
			notCreated[key] = map[string]any{"type": "invalidProperties", "description": "emailId not found"}
			continue
		}
		body, err := rt.Body(ctx, m.BlobRef)
		if err != nil {
			notCreated[key] = map[string]any{"type": "serverFail"}
			continue
		}
		if err := b.Submit(ctx, accountID, body); err != nil {
			notCreated[key] = map[string]any{"type": "forbiddenToSend", "description": err.Error()}
			continue
		}
		creationIds[key] = string(m.ID)
		created[key] = map[string]any{"id": string(m.ID)}

		// onSuccessUpdateEmail patches are keyed by the submission creationId
		// (with '#') or the email id; apply to the just-sent email.
		for k, patch := range a.OnSuccessUpdateEmail {
			if resolveRef(k, creationIds) == string(m.ID) || k == "#"+key {
				_ = applyPatch(ctx, rt, model.ID(emailID), patch)
			}
		}
	}
	return methodResult{name: "EmailSubmission/set", body: map[string]any{
		"accountId": accountID, "oldState": "0", "newState": "0",
		"created": created, "notCreated": notCreated,
	}}
}

func applyPatch(ctx context.Context, rt *account.Runtime, id model.ID, patch map[string]json.RawMessage) error {
	for path, raw := range patch {
		switch {
		case path == "keywords":
			var kw map[string]bool
			if err := json.Unmarshal(raw, &kw); err != nil {
				return err
			}
			if err := setAllKeywords(ctx, rt, id, kw); err != nil {
				return err
			}
		case strings.HasPrefix(path, "keywords/"):
			kw := strings.TrimPrefix(path, "keywords/")
			f := flagForKeyword(kw)
			if f == "" {
				continue
			}
			if err := rt.SetFlag(ctx, id, f, jsonTruthy(raw)); err != nil {
				return err
			}
		case path == "mailboxIds":
			var mids map[string]bool
			if err := json.Unmarshal(raw, &mids); err != nil {
				return err
			}
			if err := setAllMailboxes(ctx, rt, id, mids); err != nil {
				return err
			}
		case strings.HasPrefix(path, "mailboxIds/"):
			lid := model.LabelID(strings.TrimPrefix(path, "mailboxIds/"))
			if jsonTruthy(raw) {
				if err := rt.Label(ctx, id, lid); err != nil {
					return err
				}
			} else {
				if err := rt.Unlabel(ctx, id, lid); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func setAllKeywords(ctx context.Context, rt *account.Runtime, id model.ID, kw map[string]bool) error {
	m, ok := rt.Message(id)
	if !ok {
		return nil
	}
	// Clear flags not present, set those present.
	for _, f := range []model.Flag{model.FlagSeen, model.FlagFlagged, model.FlagAnswered, model.FlagDraft} {
		want := kw[keywordForFlag(f)]
		if m.Flags[f] != want {
			if err := rt.SetFlag(ctx, id, f, want); err != nil {
				return err
			}
		}
	}
	return nil
}

func setAllMailboxes(ctx context.Context, rt *account.Runtime, id model.ID, mids map[string]bool) error {
	m, ok := rt.Message(id)
	if !ok {
		return nil
	}
	for l := range m.Labels {
		if !mids[string(l)] {
			if err := rt.Unlabel(ctx, id, l); err != nil {
				return err
			}
		}
	}
	for l, on := range mids {
		if on {
			if err := rt.Label(ctx, id, model.LabelID(l)); err != nil {
				return err
			}
		}
	}
	return nil
}

func jsonTruthy(raw json.RawMessage) bool {
	var v any
	_ = json.Unmarshal(raw, &v)
	switch x := v.(type) {
	case bool:
		return x
	case nil:
		return false
	default:
		return true
	}
}

func keywordForFlag(f model.Flag) string {
	switch f {
	case model.FlagSeen:
		return "$seen"
	case model.FlagFlagged:
		return "$flagged"
	case model.FlagAnswered:
		return "$answered"
	case model.FlagDraft:
		return "$draft"
	}
	return ""
}

func flagForKeyword(kw string) model.Flag {
	switch kw {
	case "$seen":
		return model.FlagSeen
	case "$flagged":
		return model.FlagFlagged
	case "$answered":
		return model.FlagAnswered
	case "$draft":
		return model.FlagDraft
	}
	return ""
}
