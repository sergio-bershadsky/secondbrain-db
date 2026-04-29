package sbdb

import "errors"

var (
	// ErrNotFound is returned by Repo.Get/Update/Delete when the id is unknown.
	ErrNotFound = errors.New("sbdb: not found")
	// ErrSchemaInvalid is returned when a schema file fails validation at Open time.
	ErrSchemaInvalid = errors.New("sbdb: schema invalid")
	// ErrValidation is returned when a Doc fails the entity's schema validation.
	ErrValidation = errors.New("sbdb: validation failed")
	// ErrIntegrity is returned (wrapping *IntegrityError) when sidecar verification fails.
	ErrIntegrity = errors.New("sbdb: integrity violation")
	// ErrConflict is returned by Repo.Create when the id already exists.
	ErrConflict = errors.New("sbdb: id conflict")
	// ErrUnknownEntity is returned by RepoErr for an unknown schema name.
	ErrUnknownEntity = errors.New("sbdb: unknown entity")
)
