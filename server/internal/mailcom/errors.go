package mailcom

import "fmt"

type Error struct {
	Message string
	Status  int
	Kind    string
}

func (e *Error) Error() string {
	if e.Status > 0 {
		return fmt.Sprintf("%s (%d)", e.Message, e.Status)
	}
	return e.Message
}

func authError(message string) error {
	return &Error{Message: message, Kind: "auth"}
}

func apiError(message string, status int) error {
	return &Error{Message: message, Status: status, Kind: "api"}
}

func validationError(message string) error {
	return &Error{Message: message, Kind: "validation"}
}
