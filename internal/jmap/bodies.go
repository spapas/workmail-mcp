package jmap

import (
	"strings"

	maildomain "github.com/spapas/workmail-mcp/internal/mail"
)

// parseBodies converts JMAP body values and attachments to bounded text,
// HTML, and attachment metadata. Payload bytes stay server-side until
// Blob/get; oversize attachments are flagged TooLarge instead.
func parseBodies(msg email, bodyLimit, attachLimit int64) (maildomain.ParsedMessage, error) {
	var out maildomain.ParsedMessage
	out.References = append([]string(nil), msg.References...)

	text, truncated := joinValues(msg, msg.TextBody, bodyLimit)
	out.Text = text
	out.Truncated = truncated

	html, truncHTML := joinValues(msg, msg.HTMLBody, bodyLimit-int64(len(text)))
	out.HTML = html
	out.Truncated = out.Truncated || truncHTML

	for _, a := range msg.Attachments {
		meta := maildomain.AttachmentMeta{
			Index:       len(out.Attachments),
			Filename:    a.Name,
			ContentType: a.Type,
			Size:        a.Size,
			TooLarge:    a.Size > attachLimit,
			ID:          a.BlobID,
		}
		out.Attachments = append(out.Attachments, maildomain.ParsedAttachment{Meta: meta})
	}

	return out, nil
}

// joinValues concatenates body values for the given parts, bounded and
// rune-aligned. Any server-side truncation marks the result truncated.
func joinValues(msg email, parts []bodyPart, limit int64) (string, bool) {
	var sb strings.Builder
	truncated := false

	for _, part := range parts {
		val, ok := msg.BodyValues[part.PartID]
		if !ok {
			continue
		}

		if val.IsTruncated {
			truncated = true
		}

		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}

		sb.WriteString(val.Value)
	}

	joined := sb.String()
	if limit <= 0 && joined != "" {
		return "", true
	}

	if int64(len(joined)) > limit {
		return cutStr(joined, limit), true
	}

	return joined, truncated
}
