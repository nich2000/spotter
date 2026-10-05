package core

import (
	"bytes"
	"encoding/json"
	"io"
)

// Decode rejects duplicate keys, unknown struct fields and trailing JSON values.
func Decode(raw []byte, dest any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	if err := scanValue(d); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return Fail("INVALID_JSON", 400, "Лишние JSON данные")
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		return Fail("INVALID_JSON", 400, "Неверный JSON или неизвестное поле")
	}
	return nil
}
func scanValue(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return Fail("INVALID_JSON", 400, "Неверный JSON")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return Fail("INVALID_JSON", 400, "Неверный ключ")
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return Fail("INVALID_JSON", 400, "Повторный JSON ключ")
			}
			seen[key] = true
			if e = scanValue(d); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e := scanValue(d); e != nil {
				return e
			}
		}
	default:
		return Fail("INVALID_JSON", 400, "Неверный JSON")
	}
	_, err = d.Token()
	if err != nil {
		return Fail("INVALID_JSON", 400, "Незавершённый JSON")
	}
	return nil
}
