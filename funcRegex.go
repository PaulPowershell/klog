package main

func IsError(line string) bool {
	for _, re := range errorRegexps {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func IsWarning(line string) bool {
	for _, re := range warningRegexps {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func IsPanic(line string) bool {
	for _, re := range panicRegexps {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}

func IsDebug(line string) bool {
	for _, re := range debugRegexps {
		if re.MatchString(line) {
			return true
		}
	}
	return false
}
