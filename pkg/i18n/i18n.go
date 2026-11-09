package i18n

const (
	LangRU = 0
	LangEN = 1
)

var CurrentLang = LangRU

func SetLang(lang int) {
	CurrentLang = lang
}

func ToggleLang() int {
	if CurrentLang == LangRU {
		CurrentLang = LangEN
	} else {
		CurrentLang = LangRU
	}
	return CurrentLang
}

func T(ruText, enText string) string {
	if CurrentLang == LangEN {
		return enText
	}
	return ruText
}
