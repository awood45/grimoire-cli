package docgen

import (
	"bytes"
	"text/template"

	"github.com/awood45/grimoire-cli/internal/sberrors"
	"github.com/awood45/grimoire-cli/templates"
)

// Generator renders the grimoire.md document from data.
type Generator interface {
	Generate(data *DocData) (string, error)
}

// TemplateGenerator implements Generator using Go text/template.
type TemplateGenerator struct {
	tmpl *template.Template
}

// Compile-time interface check.
var _ Generator = (*TemplateGenerator)(nil)

// NewTemplateGenerator creates a TemplateGenerator from the embedded template.
func NewTemplateGenerator() (*TemplateGenerator, error) {
	tmplBytes, err := templates.FS.ReadFile("grimoire.md.tmpl")
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to read embedded template")
	}

	return newGeneratorFromBytes(tmplBytes)
}

// newGeneratorFromBytes parses template content and returns a TemplateGenerator.
func newGeneratorFromBytes(data []byte) (*TemplateGenerator, error) {
	tmpl, err := template.New("grimoire.md").Parse(string(data))
	if err != nil {
		return nil, sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to parse template")
	}

	return &TemplateGenerator{tmpl: tmpl}, nil
}

// Generate renders the grimoire.md document with the given data.
func (g *TemplateGenerator) Generate(data *DocData) (string, error) {
	var buf bytes.Buffer
	if err := g.tmpl.Execute(&buf, data); err != nil {
		return "", sberrors.Wrap(err, sberrors.ErrCodeInternalError, "failed to execute template")
	}
	return buf.String(), nil
}
