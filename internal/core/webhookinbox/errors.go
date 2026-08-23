package webhookinbox

import "errors"

var ErrIdentityConflict = errors.New("webhook delivery identity conflicts with stored payload")
var ErrInvalidDelivery = errors.New("webhook delivery is invalid")
var ErrLeaseLost = errors.New("webhook delivery lease is not owned")
