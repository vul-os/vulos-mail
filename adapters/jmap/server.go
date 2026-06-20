// Package jmap is the JMAP (RFC 8620/8621) protocol edge — the modern native
// protocol. It hand-rolls the HTTP/JSON surface over account.Runtime, mapping
// the domain model directly: labels are Mailboxes, flags+label-membership are
// Email keywords/mailboxIds. This is the protocol the webmail speaks.
//
// Scope this wave: Session, Mailbox/get, Email/query (inMailbox filter),
// Email/get, Email/set (keywords + mailboxIds patch). EmailSubmission/Thread and
// push are later refinements.
package jmap

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/mime"
	"github.com/vul-os/vmail/internal/model"
)

// Backend resolves HTTP Basic credentials to an account runtime + account id.
type Backend struct {
	Auth func(username, password string) (*account.Runtime, error)
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
		rt, err := b.Auth(user, pass)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		fn(w, r, rt, user)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- Session resource (RFC 8620 §2) ---

func (b *Backend) session(w http.ResponseWriter, _ *http.Request, _ *account.Runtime, accountID string) {
	writeJSON(w, map[string]any{
		"capabilities": map[string]any{
			"urn:ietf:params:jmap:core": map[string]any{"maxObjectsInGet": 1000, "maxObjectsInSet": 1000},
			"urn:ietf:params:jmap:mail": map[string]any{},
		},
		"accounts": map[string]any{
			accountID: map[string]any{
				"name":                accountID,
				"isPersonal":          true,
				"isReadOnly":          false,
				"accountCapabilities": map[string]any{"urn:ietf:params:jmap:mail": map[string]any{}},
			},
		},
		"primaryAccounts": map[string]any{
			"urn:ietf:params:jmap:mail": accountID,
		},
		"apiUrl":       "/jmap/api",
		"downloadUrl":  "/jmap/download/{accountId}/{blobId}",
		"uploadUrl":    "/jmap/upload/{accountId}",
		"eventSourceUrl": "/jmap/eventsource",
		"state":        "0",
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
	for _, mc := range req.MethodCalls {
		var triple []json.RawMessage
		if err := json.Unmarshal(mc, &triple); err != nil || len(triple) != 3 {
			continue
		}
		var name, callID string
		_ = json.Unmarshal(triple[0], &name)
		_ = json.Unmarshal(triple[2], &callID)
		args := triple[1]

		result := b.dispatch(rt, accountID, name, args)
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

func (b *Backend) dispatch(rt *account.Runtime, accountID, name string, args json.RawMessage) methodResult {
	switch name {
	case "Mailbox/get":
		return mailboxGet(rt, accountID)
	case "Email/query":
		return emailQuery(rt, accountID, args)
	case "Email/get":
		return emailGet(rt, accountID, args)
	case "Email/set":
		return emailSet(rt, accountID, args)
	default:
		return methodError("unknownMethod")
	}
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
			"id":            string(l.ID),
			"name":          l.Name,
			"role":          mailboxRole(l.ID),
			"totalEmails":   len(msgs),
			"unreadEmails":  unread,
			"myRights":      map[string]bool{"mayReadItems": true, "mayAddItems": true, "maySetKeywords": true},
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
		if p == "preview" || p == "bodyValues" || p == "textBody" {
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

func emailSet(rt *account.Runtime, accountID string, args json.RawMessage) methodResult {
	var a struct {
		Update map[string]map[string]json.RawMessage `json:"update"`
	}
	_ = json.Unmarshal(args, &a)

	ctx := context.Background()
	updated := map[string]any{}
	notUpdated := map[string]any{}
	// Deterministic order for testability.
	ids := make([]string, 0, len(a.Update))
	for id := range a.Update {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		patch := a.Update[id]
		if _, ok := rt.Message(model.ID(id)); !ok {
			notUpdated[id] = map[string]any{"type": "notFound"}
			continue
		}
		if err := applyPatch(ctx, rt, model.ID(id), patch); err != nil {
			notUpdated[id] = map[string]any{"type": "invalidPatch"}
			continue
		}
		updated[id] = nil
	}
	return methodResult{name: "Email/set", body: map[string]any{
		"accountId": accountID, "oldState": "0", "newState": "0",
		"updated": updated, "notUpdated": notUpdated,
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
