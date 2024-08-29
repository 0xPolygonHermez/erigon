package hermez_db

import "errors"

var ErrorNotStored = errors.New("error: not stored")
var ErrorL2BlockNumberNotMatch = errors.New("error: l2BlockNumber doesn't match")