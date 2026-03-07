// internal/services/templatesvc/errors.go
package templatesvc

import "errors"

var (
	ErrTemplateNotFound  = errors.New("template not found")
	ErrTemplateDuplicate = errors.New("template with same name already exists")
)
