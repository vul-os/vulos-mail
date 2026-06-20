// Package model holds the protocol-free domain types. It is the leaf package:
// it imports nothing else in the tree, so event/blob/projection can all depend
// on it without cycles. Nothing here knows about IMAP/JMAP/SMTP — those are
// adapters that project these types onto the wire.
package model

import "time"

// Core identifier types. IDs are opaque (ULID strings); refs are content hashes.
type (
	ID      string // internal message/thread id (ULID)
	BlobRef string // content-addressed blob ref, e.g. "sha256:abcd..."
	LabelID string // stable label id
)

// Flag is a per-message boolean that is NOT a label. Star and Important are
// deliberately labels, not flags, so they participate in the many-to-many model.
type Flag string

const (
	FlagSeen     Flag = "seen"
	FlagAnswered Flag = "answered"
	FlagFlagged  Flag = "flagged"
	FlagDraft    Flag = "draft"
	FlagMDNSent  Flag = "mdn-sent"
)

// LabelKind classifies a label.
type LabelKind string

const (
	LabelSystem   LabelKind = "system"
	LabelUser     LabelKind = "user"
	LabelCategory LabelKind = "category"
)

// System label ids (stable, well-known). Folders are projections of these.
const (
	LabelInbox     LabelID = "inbox"
	LabelArchive   LabelID = "archive"
	LabelSent      LabelID = "sent"
	LabelDrafts    LabelID = "drafts"
	LabelTrash     LabelID = "trash"
	LabelSpam      LabelID = "spam"
	LabelStar      LabelID = "star"
	LabelImportant LabelID = "important"
	LabelSnoozed   LabelID = "snoozed"
)

// SystemLabels returns the built-in labels every account starts with.
func SystemLabels() []Label {
	return []Label{
		{ID: LabelInbox, Name: "Inbox", Kind: LabelSystem},
		{ID: LabelArchive, Name: "Archive", Kind: LabelSystem},
		{ID: LabelSent, Name: "Sent", Kind: LabelSystem},
		{ID: LabelDrafts, Name: "Drafts", Kind: LabelSystem},
		{ID: LabelTrash, Name: "Trash", Kind: LabelSystem},
		{ID: LabelSpam, Name: "Spam", Kind: LabelSystem},
		{ID: LabelStar, Name: "Starred", Kind: LabelSystem},
		{ID: LabelImportant, Name: "Important", Kind: LabelSystem},
		{ID: LabelSnoozed, Name: "Snoozed", Kind: LabelSystem},
	}
}

// Envelope is the parsed, protocol-free header summary of a message.
type Envelope struct {
	From            []string
	FromName        string // display name of the first From address, if any
	To              []string
	Cc              []string
	Subject         string
	Date            time.Time
	MessageIDHeader string
	InReplyTo       string
	References      []string
}

// Message is the projected state of one message. Body lives in the blob store;
// this carries everything needed to list/search/serve without fetching it.
type Message struct {
	ID       ID
	BlobRef  BlobRef
	Envelope Envelope
	Size     int64
	ThreadID ID
	Labels   map[LabelID]bool
	Flags    map[Flag]bool
}

// Label is a tag. Many-to-many with messages — this is the Gmail shape.
type Label struct {
	ID   LabelID
	Name string
	Kind LabelKind
}

// Thread groups messages by reference chain. First-class everywhere.
type Thread struct {
	ID         ID
	MessageIDs []ID // in ingest order
}
