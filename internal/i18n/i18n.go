// Package i18n shows GitPersona's messages in English or Simplified Chinese.
//
// A message is written once, in English, as a format string; the format
// string is also the key of its translation. Text is translated where it is
// shown, so a message keeps its arguments until then:
//
//   - [T] formats a message in a language now (for output built per request);
//   - [M] keeps a message to be shown later in any language;
//   - [Errorf] makes an error whose text can be shown in any language, and
//     that wraps errors like fmt.Errorf with %w.
//
// The command line shows English; the configuration page shows the language
// the page asks for. Content written to files is never translated, so a
// change made in either language writes the same bytes.
package i18n

import (
	"errors"
	"fmt"
	"strings"
)

// Lang is a language messages can be shown in.
type Lang string

// The languages.
const (
	EN Lang = "en"
	ZH Lang = "zh"
)

// Parse returns the language a tag such as "zh-CN", "zh-Hans" or "en-US"
// names; anything that is not Chinese is English.
func Parse(tag string) Lang {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "zh" || strings.HasPrefix(tag, "zh-") || strings.HasPrefix(tag, "zh_") {
		return ZH
	}
	return EN
}

// catalogs holds the translations of each language but English, keyed by
// the English format string.
var catalogs = map[Lang]map[string]string{ZH: {}}

// register adds translations; a format registered twice is a mistake.
func register(l Lang, entries map[string]string) {
	for format, translated := range entries {
		if _, dup := catalogs[l][format]; dup {
			panic("i18n: " + string(l) + " translation registered twice: " + format)
		}
		catalogs[l][format] = translated
	}
}

// Message is a message in English with its arguments, shown in any language.
type Message struct {
	format string
	args   []any
}

// M keeps a message to be shown later.
func M(format string, args ...any) Message { return Message{format: format, args: args} }

// String is the message in English.
func (m Message) String() string { return m.In(EN) }

// In is the message in l. Arguments that are messages, or errors made with
// Errorf, are shown in l too.
func (m Message) In(l Lang) string {
	format := m.format
	if translated, ok := catalogs[l][format]; ok {
		format = translated
	}
	args := make([]any, len(m.args))
	for i, a := range m.args {
		args[i] = localized(l, a)
	}
	// %w only means something to fmt.Errorf; here it shows the error's text.
	return fmt.Sprintf(strings.ReplaceAll(format, "%w", "%v"), args...)
}

// T formats a message in l.
func T(l Lang, format string, args ...any) string { return M(format, args...).In(l) }

// messageError is an error whose text is a Message.
type messageError struct {
	msg     Message
	wrapped []error
}

// Errorf makes an error like fmt.Errorf, whose text can be shown in any
// language with Text.
func Errorf(format string, args ...any) error {
	e := &messageError{msg: M(format, args...)}
	if strings.Contains(format, "%w") {
		for _, a := range args {
			if err, ok := a.(error); ok {
				e.wrapped = append(e.wrapped, err)
			}
		}
	}
	return e
}

func (e *messageError) Error() string    { return e.msg.In(EN) }
func (e *messageError) In(l Lang) string { return e.msg.In(l) }
func (e *messageError) Unwrap() []error  { return e.wrapped }

// Text is v (a message, an error or anything else) shown in l. An error made
// with fmt.Errorf keeps its English text; one made with Errorf, or wrapping
// such an error with %w, is translated.
func Text(l Lang, v any) string {
	if s, ok := localized(l, v).(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// shownIn is what can be shown in a language.
type shownIn interface{ In(Lang) string }

func localized(l Lang, v any) any {
	if s, ok := v.(shownIn); ok {
		return s.In(l)
	}
	var me *messageError
	if err, ok := v.(error); ok && errors.As(err, &me) && err.Error() == me.Error() {
		return me.In(l) // a fmt wrapper that adds nothing to the text
	}
	return v
}
