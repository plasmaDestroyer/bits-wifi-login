package creds

import (
	"errors"
	"strings"
)

type Creds struct {
	Username, Password string
}

func Parse(content string) (Creds, error) {
	creds := Creds{}

	content = strings.TrimPrefix(content, "\ufeff")

	for line := range strings.SplitSeq(content, "\n") {
		parts := strings.SplitN(line, "=", 2)

		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = unescape(value[1 : len(value)-1])
		} else if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}

		switch key {
		case "USERNAME":
			creds.Username = value
		case "PASSWORD":
			creds.Password = value
		}
	}

	if creds.Username == "" || creds.Password == "" {
		return Creds{}, errors.New("creds: USERNAME and PASSWORD must be set")
	}

	return creds, nil
}
