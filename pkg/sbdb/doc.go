package sbdb

// Doc is the value type consumers pass to Repo methods. It is plain data:
// no hidden filesystem coupling. Repo.Create/Update return a new Doc with
// the SHA fields populated; consumers can read those for integrity audits.
type Doc struct {
	ID          string
	Frontmatter map[string]any
	Content     string
	SHA         struct {
		Content     string
		Frontmatter string
		Record      string
	}
}
