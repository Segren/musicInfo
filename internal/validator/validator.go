package validator

type Validator struct {
	Errors map[string]string
}

// создает новый валидатор с пустой картой ошибок
func New() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

func (v *Validator) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

func (v *Validator) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

// возвращает true если Errors map не содержит ошибок
func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}
