package validator

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	enTranslations "github.com/go-playground/validator/v10/translations/en"
)

var (
	validate *validator.Validate
	trans    ut.Translator
)

// usernamePattern matches letters, digits, and underscores only.
var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

func init() {
	validate = validator.New()

	// Use the json tag name in error messages (e.g. "username" not "Username").
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// English translator for human-readable default messages on built-in tags.
	enLocale := en.New()
	uni := ut.New(enLocale, enLocale)
	trans, _ = uni.GetTranslator("en")
	_ = enTranslations.RegisterDefaultTranslations(validate, trans)

	// Custom tag: alphanumeric + underscore (no built-in tag covers underscore).
	_ = validate.RegisterValidation("alphanum_underscore", func(fl validator.FieldLevel) bool {
		return usernamePattern.MatchString(fl.Field().String())
	})
	_ = validate.RegisterTranslation("alphanum_underscore", trans,
		func(t ut.Translator) error {
			return t.Add("alphanum_underscore", "{0} may only contain letters, numbers and underscores", true)
		},
		func(t ut.Translator, fe validator.FieldError) string {
			msg, _ := t.T("alphanum_underscore", fe.Field())
			return msg
		},
	)
}

// Validate validates req's struct tags and returns the first validation error
// as a human-readable message, or nil if valid.
func Validate(req any) error {
	err := validate.Struct(req)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		for _, fe := range ve {
			return fmt.Errorf("%s", fe.Translate(trans))
		}
	}
	return fmt.Errorf("invalid request")
}
