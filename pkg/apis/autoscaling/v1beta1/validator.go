package v1beta1

import (
	"fmt"
	"github.com/ringtail/go-cron"
	"reflect"
	"strings"
)

const tagName = "validate"

type Validator interface {
	Validate(interface{}) (bool, error)
}

type DefaultValidator struct {
}

type NumberValidator struct {
	Min int
	Max int
}

type StringValidator struct {
	Min int
	Max int
}

func (v StringValidator) Validate(val interface{}) (bool, error) {
	return true, nil
}

type ExcludeDatesValidator struct {
}

func (v ExcludeDatesValidator) Validate(val interface{}) (bool, error) {
	switch i := val.(type) {
	case []string:
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		for _, item := range i {
			_, err := parser.Parse(item)
			if err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func (v DefaultValidator) Validate(val interface{}) (bool,error) {
	return true, nil
}

// get Validator struct corresponding to validate tag
func getValidatorFromTag(tag string) Validator {
	args := strings.Split(tag, ",")
	switch args[0] {
	case "excludeDatesValidator":
		validator := ExcludeDatesValidator{}
		return validator
	}
	return DefaultValidator{}
}

func Validate(s interface{}) (bool, []error){
	errs := []error{}
	pass := true
	v := reflect.ValueOf(s)
	for i := 0; i < v.NumField(); i++ {
		// Get the field tag value
		tag := strings.TrimSpace(v.Type().Field(i).Tag.Get(tagName))

		if tag == "" || tag=="-" {
			continue
		}
		// Get a validator
		validator := getValidatorFromTag(tag)

		// Perform validation
		valid, err := validator.Validate(v.Field(i).Interface())

		if !valid && err != nil {
			pass = false
			errs = append(errs, fmt.Errorf("%s %s", v.Type().Field(i).Name, err.Error()))
		}
	}
	return pass, errs
}


