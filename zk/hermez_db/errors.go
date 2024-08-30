package hermez_db

import "errors"

var ErrorNotStored = errors.New("not stored")
var ErrorL2BlockNumberNotMatch = errors.New("l2BlockNumber doesn't match")