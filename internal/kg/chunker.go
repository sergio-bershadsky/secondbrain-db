package kg

import (
	"strings"
)

// Chunk represents a text segment of a document for embedding.
type Chunk struct {
	DocID      string
	ChunkIndex int
	Text       string
	ContentSHA string
}

// DefaultMaxTokens is the target chunk size in estimated tokens.
const DefaultMaxTokens = 500

// DefaultOverlap is the overlap between chunks in estimated tokens.
const DefaultOverlap = 50

// ChunkMarkdown splits a markdown document into overlapping chunks.
// Splits on headings, then packs into windows of approximately maxTokens.
// Token count is estimated as word_count * 1.3.
func ChunkMarkdown(docID, content, contentSHA string, maxTokens int) []Chunk {
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}

	if strings.TrimSpace(content) == "" {
		return nil
	}

	sections := splitOnHeadings(content)
	var chunks []Chunk
	chunkIdx := 0
	var buffer strings.Builder
	bufferTokens := 0

	for _, section := range sections {
		sectionTokens := estimateTokens(section)

		// If a single section exceeds maxTokens, split it further by paragraphs
		if sectionTokens > maxTokens {
			if bufferTokens > 0 {
				chunks = append(chunks, Chunk{
					DocID:      docID,
					ChunkIndex: chunkIdx,
					Text:       strings.TrimSpace(buffer.String()),
					ContentSHA: contentSHA,
				})
				chunkIdx++
				buffer.Reset()
				bufferTokens = 0
			}

			subChunks := splitLargeSection(section, maxTokens)
			for _, sc := range subChunks {
				chunks = append(chunks, Chunk{
					DocID:      docID,
					ChunkIndex: chunkIdx,
					Text:       strings.TrimSpace(sc),
					ContentSHA: contentSHA,
				})
				chunkIdx++
			}
			continue
		}

		// If adding this section would exceed the budget, flush
		if bufferTokens+sectionTokens > maxTokens && bufferTokens > 0 {
			chunks = append(chunks, Chunk{
				DocID:      docID,
				ChunkIndex: chunkIdx,
				Text:       strings.TrimSpace(buffer.String()),
				ContentSHA: contentSHA,
			})
			chunkIdx++

			// Keep overlap: last N tokens worth of text
			overlapText := lastNTokens(buffer.String(), DefaultOverlap)
			buffer.Reset()
			buffer.WriteString(overlapText)
			bufferTokens = estimateTokens(overlapText)
		}

		if buffer.Len() > 0 {
			buffer.WriteString("\n\n")
		}
		buffer.WriteString(section)
		bufferTokens += sectionTokens
	}

	// Flush remaining
	if bufferTokens > 0 {
		text := strings.TrimSpace(buffer.String())
		if text != "" {
			chunks = append(chunks, Chunk{
				DocID:      docID,
				ChunkIndex: chunkIdx,
				Text:       text,
				ContentSHA: contentSHA,
			})
		}
	}

	return chunks
}

// splitOnHeadings splits markdown text at heading boundaries (# ## ### etc).
func splitOnHeadings(content string) []string {
	lines := strings.Split(content, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	if current.Len() > 0 {
		sections = append(sections, current.String())
	}

	return sections
}

// splitLargeSection splits a section that exceeds maxTokens by paragraphs.
func splitLargeSection(section string, maxTokens int) []string {
	paragraphs := strings.Split(section, "\n\n")
	var results []string
	var buffer strings.Builder
	bufferTokens := 0

	for _, para := range paragraphs {
		paraTokens := estimateTokens(para)

		if bufferTokens+paraTokens > maxTokens && bufferTokens > 0 {
			results = append(results, buffer.String())
			buffer.Reset()
			bufferTokens = 0
		}

		if buffer.Len() > 0 {
			buffer.WriteString("\n\n")
		}
		buffer.WriteString(para)
		bufferTokens += paraTokens
	}

	if buffer.Len() > 0 {
		results = append(results, buffer.String())
	}

	return results
}

// estimateTokens approximates token count as word_count * 1.3.
func estimateTokens(text string) int {
	words := len(strings.Fields(text))
	return int(float64(words) * 1.3)
}

// lastNTokens returns approximately the last n tokens worth of text.
func lastNTokens(text string, n int) string {
	words := strings.Fields(text)
	targetWords := int(float64(n) / 1.3)
	if targetWords >= len(words) {
		return text
	}
	return strings.Join(words[len(words)-targetWords:], " ")
}
