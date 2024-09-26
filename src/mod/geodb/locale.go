package geodb

import "net/http"

// GetRequesterCountryISOCode get the locale of the requester
func (s *Store) GetLocaleFromRequest(r *http.Request) (string, error) {
	cc := s.GetRequesterCountryISOCode(r)
	return GetLocaleFromCountryCode(cc), nil
}

// GetLocaleFromCountryCode get the locale given the country code
func GetLocaleFromCountryCode(cc string) string {
	//If you find your country is not in the list, please add it here
	mapCountryToLocale := map[string]string{
		"aa": "ar_AA",
		"by": "be_BY",
		"bg": "bg_BG",
		"es": "ca_ES",
		"cz": "cs_CZ",
		"dk": "da_DK",
		"ch": "de_CH",
		"de": "de_DE",
		"gr": "el_GR",
		"au": "en_AU",
		"be": "en_BE",
		"gb": "en_GB",
		"jp": "en_JP",
		"us": "en_US",
		"za": "en_ZA",
		"fi": "fi_FI",
		"ca": "fr_CA",
		"fr": "fr_FR",
		"hr": "hr_HR",
		"hu": "hu_HU",
		"is": "is_IS",
		"it": "it_IT",
		"il": "iw_IL",
		"kr": "ko_KR",
		"lt": "lt_LT",
		"lv": "lv_LV",
		"mk": "mk_MK",
		"nl": "nl_NL",
		"no": "no_NO",
		"pl": "pl_PL",
		"br": "pt_BR",
		"pt": "pt_PT",
		"ro": "ro_RO",
		"ru": "ru_RU",
		"sp": "sh_SP",
		"sk": "sk_SK",
		"sl": "sl_SL",
		"al": "sq_AL",
		"se": "sv_SE",
		"th": "th_TH",
		"tr": "tr_TR",
		"ua": "uk_UA",
		"cn": "zh_CN",
		"tw": "zh_TW",
		"hk": "zh_HK",
	}
	locale, ok := mapCountryToLocale[cc]
	if !ok {
		return "en-US"
	}

	return locale
}
