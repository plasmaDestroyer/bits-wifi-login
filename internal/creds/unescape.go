package creds

func unescape(s string) string {
	res := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case '\\':
				res = append(res, '\\')
			case 'n':
				res = append(res, '\n')
			default:
				res = append(res, s[i+1])
			}
			i++
		} else {
			res = append(res, s[i])
		}
	}
	return string(res)
}
