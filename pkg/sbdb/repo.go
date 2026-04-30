package sbdb

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/document"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/query"
	"github.com/sergio-bershadsky/secondbrain-db/pkg/sbdb/schema"
)

// Repo is the per-entity service. Stateless; safe to share across goroutines.
type Repo struct {
	db     *DB
	schema *schema.Schema
}

// Schema returns the entity schema.
func (r *Repo) Schema() *schema.Schema { return r.schema }

// Create persists a new document. The id field must be set; if a document
// with that id already exists, returns ErrConflict.
func (r *Repo) Create(ctx context.Context, doc Doc) (Doc, error) {
	if err := ctx.Err(); err != nil {
		return Doc{}, err
	}
	id, _ := doc.Frontmatter[r.schema.IDField].(string)
	if id == "" {
		return Doc{}, fmt.Errorf("%w: missing %q field", ErrValidation, r.schema.IDField)
	}
	// Check existence to enforce ErrConflict semantics.
	inner := document.New(r.schema, r.db.cfg.Root)
	inner.Data = cloneMap(doc.Frontmatter)
	inner.Content = doc.Content
	if _, err := os.Stat(inner.FilePath()); err == nil {
		return Doc{}, fmt.Errorf("%w: %q", ErrConflict, id)
	}
	if err := inner.Save(r.db.rt); err != nil {
		return Doc{}, mapErr(err)
	}
	return r.Get(ctx, id)
}

// Update reads the document with the given id, calls fn with a copy, and
// persists the result. Concurrent updates race naturally — last writer wins.
// Returns ErrNotFound if the id does not exist.
func (r *Repo) Update(ctx context.Context, id string, fn func(Doc) Doc) (Doc, error) {
	if err := ctx.Err(); err != nil {
		return Doc{}, err
	}
	cur, err := r.Get(ctx, id)
	if err != nil {
		return Doc{}, err
	}
	next := fn(cur)
	// Force the id to remain stable even if fn forgot to copy it.
	if next.Frontmatter == nil {
		next.Frontmatter = map[string]any{}
	}
	next.Frontmatter[r.schema.IDField] = id

	inner := document.New(r.schema, r.db.cfg.Root)
	inner.Data = cloneMap(next.Frontmatter)
	inner.Content = next.Content
	if err := inner.Save(r.db.rt); err != nil {
		return Doc{}, mapErr(err)
	}
	return r.Get(ctx, id)
}

// Delete removes the document and its sidecar. Returns ErrNotFound if absent.
func (r *Repo) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	inner := document.New(r.schema, r.db.cfg.Root)
	inner.Data = map[string]any{r.schema.IDField: id}
	if _, err := os.Stat(inner.FilePath()); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return err
	}
	if err := inner.Delete(); err != nil {
		return mapErr(err)
	}
	return nil
}

// Get reads the document by id.
func (r *Repo) Get(ctx context.Context, id string) (Doc, error) {
	if err := ctx.Err(); err != nil {
		return Doc{}, err
	}
	inner := document.New(r.schema, r.db.cfg.Root)
	inner.Data = map[string]any{r.schema.IDField: id}
	inner.OnSave = nil
	inner.OnDelete = nil
	if _, err := os.Stat(inner.FilePath()); err != nil {
		if os.IsNotExist(err) {
			return Doc{}, fmt.Errorf("%w: %q", ErrNotFound, id)
		}
		return Doc{}, err
	}
	loaded, err := document.LoadFromFile(r.schema, r.db.cfg.Root, inner.FilePath())
	if err != nil {
		return Doc{}, mapErr(err)
	}
	if err := loaded.EnsureLoaded(); err != nil {
		return Doc{}, mapErr(err)
	}
	return Doc{
		ID:          id,
		Frontmatter: cloneMap(loaded.Data),
		Content:     loaded.Content,
	}, nil
}

// Query returns a query builder rooted at this entity.
func (r *Repo) Query() *query.QuerySet {
	return query.NewQuerySet(r.schema, r.db.cfg.Root)
}

func cloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	// Future: introspect document/integrity errors and wrap as IntegrityError, etc.
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
