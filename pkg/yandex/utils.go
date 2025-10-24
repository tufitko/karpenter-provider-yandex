package yandex

func MatchLabels(current, wanted map[string]string) bool {
	for key, value := range wanted {
		v, ok := current[key]
		if !ok {
			return false
		}
		if value == "*" {
			continue
		}
		if v != value {
			return false
		}
	}
	return true
}
