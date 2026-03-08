// internal/services/rejectedpayloadpatternsvc/errors.go
package rejectedpayloadpatternsvc

import "github.com/hotkhwan/gateway-api/internal/repo/rejectedpayloadpatternrepo"

var ErrNotFound = rejectedpayloadpatternrepo.ErrNotFound
var ErrAlreadyExists = rejectedpayloadpatternrepo.ErrAlreadyExists
