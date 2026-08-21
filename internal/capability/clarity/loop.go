package clarity

func SpecBytesDelta(previous, current int64) (delta int64, err error) {
	delta = current - previous
	if previous > 0 && delta > 0 {
		return delta, Error{Code: CodeSpecGrew}
	}
	return delta, nil
}
