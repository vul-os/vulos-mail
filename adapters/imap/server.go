// This file wires emersion/go-imap v2's imapserver.Session over an
// account.Runtime, projecting the label model onto IMAP folders. UIDs come from
// the edge UID view (uidview.go); message rendering reuses go-imap's Extract*
// helpers and MailboxTracker/SessionTracker, so we never re-implement MIME or
// the IMAP wire format. Scope: a correct read + flag/expunge/append/copy path;
// live push (tracker fed from Runtime.onChange) and full SEARCH are later
// refinements (NOTE markers below).
package imap

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-message/textproto"

	"github.com/vul-os/vulos-mail/internal/account"
	"github.com/vul-os/vulos-mail/internal/mime"
	"github.com/vul-os/vulos-mail/internal/model"
)

const delim = '/'

// Backend resolves a login to an account runtime.
type Backend struct {
	// Auth returns the runtime for the given credentials, or an error.
	Auth func(username, password string) (*account.Runtime, error)
}

// NewServer builds a go-imap server over this backend. TLS optional (STARTTLS).
func NewServer(be *Backend, tlsConfig *tls.Config) *imapserver.Server {
	return imapserver.New(&imapserver.Options{
		NewSession:   be.newSession,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}},
		TLSConfig:    tlsConfig,
		InsecureAuth: tlsConfig == nil,
	})
}

func (b *Backend) newSession(_ *imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
	return &session{backend: b}, nil, nil
}

type selMsg struct {
	uid uint32
	id  model.ID
	msg *model.Message
}

type session struct {
	backend *Backend
	rt      *account.Runtime

	// selected-state
	selLabel    model.LabelID
	tracker     *imapserver.MailboxTracker
	sessTracker *imapserver.SessionTracker
	msgs        []selMsg
	deleted     map[model.ID]bool // \Deleted marks, cleared on (re)select
	searchRes   imap.UIDSet
	changes     <-chan struct{} // account change notifications (for IDLE push)
	unsub       func()
}

func (s *session) Close() error {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
	}
	if s.sessTracker != nil {
		s.sessTracker.Close()
	}
	return nil
}

// --- not-authenticated state ---

func (s *session) Login(username, password string) error {
	rt, err := s.backend.Auth(username, password)
	if err != nil {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeAuthenticationFailed, Text: "invalid credentials"}
	}
	s.rt = rt
	return nil
}

// --- folder<->label mapping ---

func folderName(l *model.Label) string {
	if l.ID == model.LabelInbox {
		return "INBOX"
	}
	return l.Name
}

func (s *session) resolveLabel(name string) (model.LabelID, bool) {
	if strings.EqualFold(name, "INBOX") {
		return model.LabelInbox, true
	}
	for _, l := range s.rt.Labels() {
		if strings.EqualFold(l.Name, name) || string(l.ID) == name {
			return l.ID, true
		}
	}
	return "", false
}

func (s *session) view(ctx context.Context) (*View, error) {
	recs, err := s.rt.Records(ctx)
	if err != nil {
		return nil, err
	}
	return ViewFrom(recs), nil
}

// --- authenticated state ---

func (s *session) Select(name string, _ *imap.SelectOptions) (*imap.SelectData, error) {
	ctx := context.Background()
	label, ok := s.resolveLabel(name)
	if !ok {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "no such mailbox"}
	}
	v, err := s.view(ctx)
	if err != nil {
		return nil, err
	}
	s.loadSelection(label, v)

	flags := []imap.Flag{imap.FlagSeen, imap.FlagAnswered, imap.FlagFlagged, imap.FlagDraft, imap.FlagDeleted}
	permanent := append([]imap.Flag{}, flags...)
	permanent = append(permanent, imap.FlagWildcard)
	return &imap.SelectData{
		Flags:             flags,
		PermanentFlags:    permanent,
		NumMessages:       uint32(len(s.msgs)),
		UIDNext:           imap.UID(v.UIDNext(label)),
		UIDValidity:       v.UIDValidity(label),
		FirstUnseenSeqNum: s.firstUnseen(),
	}, nil
}

func (s *session) loadSelection(label model.LabelID, v *View) {
	s.selLabel = label
	s.deleted = map[model.ID]bool{}
	s.msgs = s.msgs[:0]
	for _, e := range v.Entries(label) {
		if m, ok := s.rt.Message(e.Msg); ok {
			s.msgs = append(s.msgs, selMsg{uid: e.UID, id: e.Msg, msg: m})
		}
	}
	if s.sessTracker != nil {
		s.sessTracker.Close()
	}
	s.tracker = imapserver.NewMailboxTracker(uint32(len(s.msgs)))
	s.sessTracker = s.tracker.NewSession()
	// (Re)subscribe to account changes for IDLE push.
	if s.unsub != nil {
		s.unsub()
	}
	if s.rt != nil {
		s.changes, s.unsub = s.rt.Subscribe()
	}
}

func (s *session) firstUnseen() uint32 {
	for i, m := range s.msgs {
		if !m.msg.Flags[model.FlagSeen] {
			return uint32(i) + 1
		}
	}
	return 0
}

func (s *session) Unselect() error {
	if s.unsub != nil {
		s.unsub()
		s.unsub = nil
		s.changes = nil
	}
	if s.sessTracker != nil {
		s.sessTracker.Close()
		s.sessTracker = nil
	}
	s.tracker = nil
	s.msgs = nil
	s.selLabel = ""
	return nil
}

func (s *session) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	ctx := context.Background()
	if len(patterns) == 0 {
		// LIST "" "" is a request for the hierarchy delimiter.
		return w.WriteList(&imap.ListData{Attrs: []imap.MailboxAttr{imap.MailboxAttrNoSelect}, Delim: delim})
	}
	v, _ := s.view(ctx)
	for _, l := range s.rt.Labels() {
		name := folderName(l)
		if !matchAny(name, patterns) {
			continue
		}
		data := &imap.ListData{Mailbox: name, Delim: delim}
		if options != nil && options.ReturnStatus != nil && v != nil {
			data.Status = s.statusFor(l.ID, name, v, options.ReturnStatus)
		}
		if err := w.WriteList(data); err != nil {
			return err
		}
	}
	return nil
}

func matchAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if imapserver.MatchList(name, delim, "", p) {
			return true
		}
	}
	return false
}

func (s *session) Status(name string, options *imap.StatusOptions) (*imap.StatusData, error) {
	ctx := context.Background()
	label, ok := s.resolveLabel(name)
	if !ok {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "no such mailbox"}
	}
	v, err := s.view(ctx)
	if err != nil {
		return nil, err
	}
	return s.statusFor(label, name, v, options), nil
}

func (s *session) statusFor(label model.LabelID, name string, v *View, o *imap.StatusOptions) *imap.StatusData {
	data := &imap.StatusData{Mailbox: name}
	msgs := s.rt.MessagesWithLabel(label)
	if o.NumMessages {
		n := uint32(len(msgs))
		data.NumMessages = &n
	}
	if o.UIDNext {
		data.UIDNext = imap.UID(v.UIDNext(label))
	}
	if o.UIDValidity {
		data.UIDValidity = v.UIDValidity(label)
	}
	if o.NumUnseen {
		var n uint32
		for _, m := range msgs {
			if !m.Flags[model.FlagSeen] {
				n++
			}
		}
		data.NumUnseen = &n
	}
	return data
}

// --- selected state: fetch/store/search/expunge/copy ---

func (s *session) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	ctx := context.Background()
	markSeen := false
	for _, bs := range options.BodySection {
		if !bs.Peek {
			markSeen = true
			break
		}
	}
	var ferr error
	s.forEach(numSet, func(seqNum uint32, m *selMsg) {
		if ferr != nil {
			return
		}
		raw, err := s.rt.Body(ctx, m.msg.BlobRef)
		if err != nil {
			ferr = err
			return
		}
		if markSeen && !m.msg.Flags[model.FlagSeen] {
			_ = s.rt.SetFlag(ctx, m.id, model.FlagSeen, true)
			m.msg.Flags[model.FlagSeen] = true
		}
		rw := w.CreateMessage(s.sessTracker.EncodeSeqNum(seqNum))
		ferr = s.writeMessage(rw, m, raw, options)
	})
	return ferr
}

func (s *session) writeMessage(w *imapserver.FetchResponseWriter, m *selMsg, raw []byte, o *imap.FetchOptions) error {
	w.WriteUID(imap.UID(m.uid))
	if o.Flags {
		w.WriteFlags(toIMAPFlags(m.msg, s.deleted[m.id]))
	}
	if o.InternalDate {
		w.WriteInternalDate(m.msg.Envelope.Date)
	}
	if o.RFC822Size {
		w.WriteRFC822Size(m.msg.Size)
	}
	if o.Envelope {
		br := bufio.NewReader(bytes.NewReader(raw))
		if hdr, err := textproto.ReadHeader(br); err == nil {
			w.WriteEnvelope(imapserver.ExtractEnvelope(hdr))
		}
	}
	if o.BodyStructure != nil {
		w.WriteBodyStructure(imapserver.ExtractBodyStructure(bytes.NewReader(raw)))
	}
	for _, bs := range o.BodySection {
		buf := imapserver.ExtractBodySection(bytes.NewReader(raw), bs)
		wc := w.WriteBodySection(bs, int64(len(buf)))
		if _, err := wc.Write(buf); err != nil {
			wc.Close()
			return err
		}
		if err := wc.Close(); err != nil {
			return err
		}
	}
	return w.Close()
}

func (s *session) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	ctx := context.Background()
	s.forEach(numSet, func(_ uint32, m *selMsg) {
		s.applyStore(ctx, m, flags)
	})
	if !flags.Silent {
		return s.Fetch(w, numSet, &imap.FetchOptions{Flags: true})
	}
	return nil
}

func (s *session) applyStore(ctx context.Context, m *selMsg, st *imap.StoreFlags) {
	set := func(f imap.Flag, on bool) {
		if f == imap.FlagDeleted {
			s.deleted[m.id] = on
			return
		}
		mf, ok := modelFlag(f)
		if !ok {
			return
		}
		_ = s.rt.SetFlag(ctx, m.id, mf, on)
		m.msg.Flags[mf] = on
		if !on {
			delete(m.msg.Flags, mf)
		}
	}
	switch st.Op {
	case imap.StoreFlagsSet:
		for _, mf := range []model.Flag{model.FlagSeen, model.FlagAnswered, model.FlagFlagged, model.FlagDraft} {
			set(modelToIMAP(mf), false)
		}
		s.deleted[m.id] = false
		for _, f := range st.Flags {
			set(f, true)
		}
	case imap.StoreFlagsAdd:
		for _, f := range st.Flags {
			set(f, true)
		}
	case imap.StoreFlagsDel:
		for _, f := range st.Flags {
			set(f, false)
		}
	}
}

func (s *session) Search(numKind imapserver.NumKind, criteria *imap.SearchCriteria, options *imap.SearchOptions) (*imap.SearchData, error) {
	ctx := context.Background()
	var (
		data   imap.SearchData
		seqSet imap.SeqSet
		uidSet imap.UIDSet
	)
	for i := range s.msgs {
		seqNum := uint32(i) + 1
		m := &s.msgs[i]
		if !s.matches(ctx, seqNum, m, criteria) {
			continue
		}
		uidSet.AddNum(imap.UID(m.uid))
		var num uint32
		switch numKind {
		case imapserver.NumKindSeq:
			seqSet.AddNum(seqNum)
			num = seqNum
		case imapserver.NumKindUID:
			num = m.uid
		}
		if data.Min == 0 || num < data.Min {
			data.Min = num
		}
		if num > data.Max {
			data.Max = num
		}
		data.Count++
	}
	if numKind == imapserver.NumKindUID {
		data.All = uidSet
	} else {
		data.All = seqSet
	}
	if options != nil && options.ReturnSave {
		s.searchRes = uidSet
	}
	return &data, nil
}

func (s *session) matches(ctx context.Context, seqNum uint32, m *selMsg, c *imap.SearchCriteria) bool {
	for _, ss := range c.SeqNum {
		if !ss.Contains(seqNum) {
			return false
		}
	}
	for _, us := range c.UID {
		if !us.Contains(imap.UID(m.uid)) {
			return false
		}
	}
	for _, f := range c.Flag {
		if !hasIMAPFlag(m.msg, s.deleted[m.id], f) {
			return false
		}
	}
	for _, f := range c.NotFlag {
		if hasIMAPFlag(m.msg, s.deleted[m.id], f) {
			return false
		}
	}
	// Text/Body substring (best-effort; full SEARCH grammar is a later refinement).
	terms := append(append([]string{}, c.Body...), c.Text...)
	if len(terms) > 0 {
		raw, err := s.rt.Body(ctx, m.msg.BlobRef)
		if err != nil {
			return false
		}
		hay := strings.ToLower(string(raw))
		if text, err := mime.ExtractText(raw); err == nil {
			hay = strings.ToLower(text + " " + m.msg.Envelope.Subject)
		}
		for _, t := range terms {
			if !strings.Contains(hay, strings.ToLower(t)) {
				return false
			}
		}
	}
	return true
}

func (s *session) Expunge(w *imapserver.ExpungeWriter, uids *imap.UIDSet) error {
	ctx := context.Background()
	var keep []selMsg
	// Walk descending so emitted sequence numbers stay valid as we remove.
	toExpunge := map[int]bool{}
	for i := range s.msgs {
		m := &s.msgs[i]
		if !s.deleted[m.id] {
			continue
		}
		if uids != nil && !uids.Contains(imap.UID(m.uid)) {
			continue
		}
		toExpunge[i] = true
	}
	for i := len(s.msgs) - 1; i >= 0; i-- {
		if !toExpunge[i] {
			continue
		}
		// Gmail-style: expunge in a label removes the label (the message lives on
		// in All Mail). Trash is the exception. NOTE: Trash hard-delete is a later refinement.
		_ = s.rt.Unlabel(ctx, s.msgs[i].id, s.selLabel)
		if err := w.WriteExpunge(s.sessTracker.EncodeSeqNum(uint32(i) + 1)); err != nil {
			return err
		}
	}
	for i := range s.msgs {
		if !toExpunge[i] {
			keep = append(keep, s.msgs[i])
		}
	}
	s.msgs = keep
	// Reset trackers to the new membership (no live-update queue in this wave).
	if s.sessTracker != nil {
		s.sessTracker.Close()
	}
	s.tracker = imapserver.NewMailboxTracker(uint32(len(s.msgs)))
	s.sessTracker = s.tracker.NewSession()
	return nil
}

func (s *session) Copy(numSet imap.NumSet, dest string) (*imap.CopyData, error) {
	ctx := context.Background()
	label, ok := s.resolveLabel(dest)
	if !ok {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeTryCreate, Text: "no such mailbox"}
	}
	var sourceUIDs imap.UIDSet
	var ids []model.ID
	s.forEach(numSet, func(_ uint32, m *selMsg) {
		_ = s.rt.Label(ctx, m.id, label)
		sourceUIDs.AddNum(imap.UID(m.uid))
		ids = append(ids, m.id)
	})
	v, err := s.view(ctx)
	if err != nil {
		return nil, err
	}
	var destUIDs imap.UIDSet
	for _, id := range ids {
		if uid, ok := v.UIDOf(label, id); ok {
			destUIDs.AddNum(imap.UID(uid))
		}
	}
	return &imap.CopyData{UIDValidity: v.UIDValidity(label), SourceUIDs: sourceUIDs, DestUIDs: destUIDs}, nil
}

func (s *session) Append(mailbox string, r imap.LiteralReader, options *imap.AppendOptions) (*imap.AppendData, error) {
	ctx := context.Background()
	label, ok := s.resolveLabel(mailbox)
	if !ok {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeTryCreate, Text: "no such mailbox"}
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		return nil, err
	}
	var flags []model.Flag
	if options != nil {
		for _, f := range options.Flags {
			if mf, ok := modelFlag(f); ok {
				flags = append(flags, mf)
			}
		}
	}
	id, err := s.rt.Ingest(ctx, buf.Bytes(), []model.LabelID{label}, flags)
	if err != nil {
		return nil, err
	}
	v, err := s.view(ctx)
	if err != nil {
		return nil, err
	}
	uid, _ := v.UIDOf(label, id)
	return &imap.AppendData{UIDValidity: v.UIDValidity(label), UID: imap.UID(uid)}, nil
}

// --- mailbox management ---

func (s *session) Create(mailbox string, _ *imap.CreateOptions) error {
	id := model.LabelID(mailbox)
	return s.rt.CreateLabel(context.Background(), id, mailbox, model.LabelUser)
}

func (s *session) Delete(mailbox string) error {
	label, ok := s.resolveLabel(mailbox)
	if !ok {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "no such mailbox"}
	}
	return s.rt.DeleteLabel(context.Background(), label)
}

func (s *session) Rename(mailbox, newName string, _ *imap.RenameOptions) error {
	label, ok := s.resolveLabel(mailbox)
	if !ok {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "no such mailbox"}
	}
	return s.rt.RenameLabel(context.Background(), label, newName)
}

func (s *session) Subscribe(string) error   { return nil }
func (s *session) Unsubscribe(string) error { return nil }

// --- updates ---

func (s *session) Poll(w *imapserver.UpdateWriter, allowExpunge bool) error {
	if s.sessTracker == nil {
		return nil
	}
	return s.sessTracker.Poll(w, allowExpunge)
}

func (s *session) Idle(w *imapserver.UpdateWriter, stop <-chan struct{}) error {
	if s.sessTracker == nil {
		<-stop
		return nil
	}
	// Watch for account changes and push new mail (EXISTS) into the tracker while
	// the client idles; the SessionTracker.Idle below delivers them.
	done := make(chan struct{})
	if s.changes != nil {
		go func() {
			ctx := context.Background()
			for {
				select {
				case <-done:
					return
				case <-s.changes:
					v, err := s.view(ctx)
					if err != nil {
						continue
					}
					ents := v.Entries(s.selLabel)
					if len(ents) > len(s.msgs) {
						for _, e := range ents[len(s.msgs):] {
							if m, ok := s.rt.Message(e.Msg); ok {
								s.msgs = append(s.msgs, selMsg{uid: e.UID, id: e.Msg, msg: m})
							}
						}
						s.tracker.QueueNumMessages(uint32(len(s.msgs)))
					}
				}
			}
		}()
	}
	err := s.sessTracker.Idle(w, stop)
	close(done)
	return err
}

// --- iteration over the selection honoring seq/uid sets + SEARCHRES ---

func (s *session) forEach(numSet imap.NumSet, f func(seqNum uint32, m *selMsg)) {
	numSet = s.staticNumSet(numSet)
	for i := range s.msgs {
		seqNum := uint32(i) + 1
		m := &s.msgs[i]
		var contains bool
		switch ns := numSet.(type) {
		case imap.SeqSet:
			enc := s.sessTracker.EncodeSeqNum(seqNum)
			contains = enc != 0 && ns.Contains(enc)
		case imap.UIDSet:
			contains = ns.Contains(imap.UID(m.uid))
		}
		if contains {
			f(seqNum, m)
		}
	}
}

func (s *session) staticNumSet(numSet imap.NumSet) imap.NumSet {
	if imap.IsSearchRes(numSet) {
		return s.searchRes
	}
	maxSeq := uint32(len(s.msgs))
	var maxUID uint32
	if maxSeq > 0 {
		maxUID = s.msgs[maxSeq-1].uid
	}
	switch ns := numSet.(type) {
	case imap.SeqSet:
		for i := range ns {
			fixRange(&ns[i].Start, &ns[i].Stop, maxSeq)
		}
		return ns
	case imap.UIDSet:
		for i := range ns {
			fixRange((*uint32)(&ns[i].Start), (*uint32)(&ns[i].Stop), maxUID)
		}
		return ns
	}
	return numSet
}

func fixRange(start, stop *uint32, max uint32) {
	dyn := false
	if *start == 0 {
		*start = max
		dyn = true
	}
	if *stop == 0 {
		*stop = max
		dyn = true
	}
	if dyn && *start > *stop {
		*start, *stop = *stop, *start
	}
}

// --- flag mapping ---

func toIMAPFlags(m *model.Message, deleted bool) []imap.Flag {
	var f []imap.Flag
	if m.Flags[model.FlagSeen] {
		f = append(f, imap.FlagSeen)
	}
	if m.Flags[model.FlagAnswered] {
		f = append(f, imap.FlagAnswered)
	}
	if m.Flags[model.FlagFlagged] {
		f = append(f, imap.FlagFlagged)
	}
	if m.Flags[model.FlagDraft] {
		f = append(f, imap.FlagDraft)
	}
	if deleted {
		f = append(f, imap.FlagDeleted)
	}
	return f
}

func hasIMAPFlag(m *model.Message, deleted bool, f imap.Flag) bool {
	if f == imap.FlagDeleted {
		return deleted
	}
	mf, ok := modelFlag(f)
	return ok && m.Flags[mf]
}

func modelFlag(f imap.Flag) (model.Flag, bool) {
	switch f {
	case imap.FlagSeen:
		return model.FlagSeen, true
	case imap.FlagAnswered:
		return model.FlagAnswered, true
	case imap.FlagFlagged:
		return model.FlagFlagged, true
	case imap.FlagDraft:
		return model.FlagDraft, true
	}
	return "", false
}

func modelToIMAP(mf model.Flag) imap.Flag {
	switch mf {
	case model.FlagSeen:
		return imap.FlagSeen
	case model.FlagAnswered:
		return imap.FlagAnswered
	case model.FlagFlagged:
		return imap.FlagFlagged
	case model.FlagDraft:
		return imap.FlagDraft
	}
	return imap.Flag(mf)
}
