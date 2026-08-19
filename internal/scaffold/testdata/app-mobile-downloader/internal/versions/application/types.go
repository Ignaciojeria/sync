package versionsapp

import "time"

// Version representa una versión desplegada de la app: un commit
// de merge en la rama principal producto de aceptar un worktree.
// Tras el merge, air recompila y el binario pasa a correr con
// el código de ese commit, por eso cada merge = nueva versión.
type Version struct {
	SHA         string    `json:"sha"`
	ShortSHA    string    `json:"short_sha"`
	Message     string    `json:"message"`
	Author      string    `json:"author"`
	AuthorEmail string    `json:"author_email"`
	When        time.Time `json:"when"`
	Branch      string    `json:"branch"`
	IsCurrent   bool      `json:"is_current"`
}

// VersionFile representa un archivo cambiado entre una versión y
// HEAD. Se usa en la vista de detalle para mostrar el diff.
type VersionFile struct {
	Path    string `json:"path"`
	Adds    int    `json:"adds"`
	Dels    int    `json:"dels"`
	Binary  bool   `json:"binary"`
	Preview string `json:"preview"`
}