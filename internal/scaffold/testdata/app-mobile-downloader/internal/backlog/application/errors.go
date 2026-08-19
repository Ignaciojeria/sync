package application

import "errors"

// ErrNotFound se devuelve cuando una tarjeta no existe. El HTTP lo
// traduce a 404.
var ErrNotFound = errors.New("backlog: card not found")

// ErrInvalidInput se devuelve cuando los datos recibidos no pasan la
// validación del SPEC. El HTTP lo traduce a 400.
var ErrInvalidInput = errors.New("backlog: invalid input")

// ErrConflict se devuelve cuando una operación no se puede completar
// por un conflicto con el estado actual (ej: slug colisiona y el
// caller pidió strict). El HTTP lo traduce a 409.
var ErrConflict = errors.New("backlog: conflict")
