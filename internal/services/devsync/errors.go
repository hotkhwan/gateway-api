// internal/services/devsync/errors.go
package devsync

import "errors"

var ErrEdgeNotFound = errors.New("edge config not found")
var ErrEdgeNotSVMS = errors.New("edge type is not svms")
