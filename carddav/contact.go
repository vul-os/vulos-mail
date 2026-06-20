package carddav

import (
	"bytes"

	"github.com/emersion/go-vcard"
)

// Contact is a minimal contact projection of a vCard (the webmail's model).
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ParseContact extracts name + preferred email from a vCard body.
func ParseContact(data []byte) Contact {
	dec := vcard.NewDecoder(bytes.NewReader(data))
	card, err := dec.Decode()
	if err != nil {
		return Contact{}
	}
	return Contact{
		Name:  card.PreferredValue(vcard.FieldFormattedName),
		Email: card.PreferredValue(vcard.FieldEmail),
	}
}

// BuildContact serializes a contact to a vCard 4.0 body.
func BuildContact(c Contact) ([]byte, error) {
	card := vcard.Card{}
	card.SetValue(vcard.FieldFormattedName, c.Name)
	card.SetValue(vcard.FieldEmail, c.Email)
	vcard.ToV4(card)
	var buf bytes.Buffer
	if err := vcard.NewEncoder(&buf).Encode(card); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
