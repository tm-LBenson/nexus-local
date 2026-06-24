package app

import (
	"crypto/rand"
	"encoding/hex"
	"io"

	"github.com/tm-lbenson/nexus-local/services/api/internal/domain"
)

type RandomIDs struct {
	reader io.Reader
}

func NewRandomIDs() RandomIDs {
	return RandomIDs{reader: rand.Reader}
}

func (g RandomIDs) NewDocumentID() domain.DocumentID {
	return domain.DocumentID("doc_" + randomHex(g.reader, 16))
}

func (g RandomIDs) NewTenantID() domain.TenantID {
	return domain.TenantID("tenant_" + randomHex(g.reader, 12))
}

func (g RandomIDs) NewJobID() domain.JobID {
	return domain.JobID("job_" + randomHex(g.reader, 16))
}

func (g RandomIDs) NewConversationID() domain.ConversationID {
	return domain.ConversationID("conv_" + randomHex(g.reader, 16))
}

func (g RandomIDs) NewMessageID() domain.MessageID {
	return domain.MessageID("msg_" + randomHex(g.reader, 16))
}

func randomHex(reader io.Reader, bytesLen int) string {
	buf := make([]byte, bytesLen)
	if _, err := io.ReadFull(reader, buf); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}
