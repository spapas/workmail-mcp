package mail

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"

	_ "github.com/emersion/go-message/charset"
	msgmail "github.com/emersion/go-message/mail"
)

// ParsedAttachment is an attachment produced by MIME parsing.
type ParsedAttachment struct {
	Meta AttachmentMeta
	Data []byte
}

// ParsedMessage is the bounded result of MIME parsing.
type ParsedMessage struct {
	Text        string
	HTML        string
	References  []string
	Attachments []ParsedAttachment
	Truncated   bool
}

// ParseRaw parses a bounded RFC 822 message and decodes MIME transfer encodings.
func ParseRaw(raw []byte, bodyLimit, attachmentLimit int64) (ParsedMessage, error) {
	r, err := msgmail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return ParsedMessage{}, fmt.Errorf("parse message: %w", err)
	}
	defer r.Close()

	var out ParsedMessage
	if refs, err := r.Header.MsgIDList("References"); err == nil {
		out.References = refs
	}

	for {
		part, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ParsedMessage{}, fmt.Errorf("parse MIME part: %w", err)
		}

		contentType, ctParams := parseMediaType(part.Header.Get("Content-Type"), "text/plain")
		disposition, dispParams := parseMediaType(part.Header.Get("Content-Disposition"), "")
		filename := dispParams["filename"]
		if filename == "" {
			filename = ctParams["name"]
		}

		isAttachment := strings.EqualFold(disposition, "attachment") || filename != "" || !strings.HasPrefix(strings.ToLower(contentType), "text/")
		if isAttachment {
			data, err := io.ReadAll(part.Body)
			if err != nil {
				return ParsedMessage{}, fmt.Errorf("read attachment: %w", err)
			}
			meta := AttachmentMeta{
				Index:       len(out.Attachments),
				Filename:    filename,
				ContentType: contentType,
				Size:        int64(len(data)),
				TooLarge:    int64(len(data)) > attachmentLimit,
			}
			if meta.TooLarge {
				data = nil
			}
			out.Attachments = append(out.Attachments, ParsedAttachment{Meta: meta, Data: data})
			continue
		}

		data, truncated, err := readBounded(part.Body, bodyLimit)
		if err != nil {
			return ParsedMessage{}, fmt.Errorf("read body: %w", err)
		}
		out.Truncated = out.Truncated || truncated
		switch strings.ToLower(contentType) {
		case "text/plain":
			out.Text = appendBounded(out.Text, string(data), bodyLimit, &out.Truncated)
		case "text/html":
			out.HTML = appendBounded(out.HTML, string(data), bodyLimit, &out.Truncated)
		}
	}
	return out, nil
}

func parseMediaType(raw, fallback string) (string, map[string]string) {
	if strings.TrimSpace(raw) == "" {
		return fallback, map[string]string{}
	}
	mediaType, params, err := mime.ParseMediaType(raw)
	if err != nil {
		return fallback, map[string]string{}
	}
	return mediaType, params
}

func readBounded(r io.Reader, limit int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

func appendBounded(dst, src string, limit int64, truncated *bool) string {
	if dst != "" && src != "" {
		src = "\n\n" + src
	}
	remaining := limit - int64(len(dst))
	if remaining <= 0 {
		if src != "" {
			*truncated = true
		}
		return dst
	}
	if int64(len(src)) > remaining {
		*truncated = true
		return dst + src[:remaining]
	}
	return dst + src
}

// AttachmentMetadata returns attachment descriptions without payload bytes.
func (p ParsedMessage) AttachmentMetadata() []AttachmentMeta {
	out := make([]AttachmentMeta, 0, len(p.Attachments))
	for _, a := range p.Attachments {
		out = append(out, a.Meta)
	}
	return out
}
