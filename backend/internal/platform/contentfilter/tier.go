package contentfilter

// ParseTier 把配置里的字符串档位解析成 Tier；不认识返回 false。
func ParseTier(raw string) (Tier, bool) {
	switch Tier(raw) {
	case TierBlock:
		return TierBlock, true
	case TierWatch:
		return TierWatch, true
	}
	return "", false
}

// ParseCategory 把配置里的字符串类目解析成 Category；不认识返回 false。
func ParseCategory(raw string) (Category, bool) {
	switch Category(raw) {
	case CategoryDrugs:
		return CategoryDrugs, true
	case CategoryGambling:
		return CategoryGambling, true
	case CategoryFraud:
		return CategoryFraud, true
	case CategoryPorn:
		return CategoryPorn, true
	case CategoryFakeDocs:
		return CategoryFakeDocs, true
	case CategoryPolitics:
		return CategoryPolitics, true
	case CategoryViolence:
		return CategoryViolence, true
	}
	return "", false
}

// Valid 判断档位是否是登记过的两档之一。
func (t Tier) Valid() bool {
	_, ok := ParseTier(string(t))
	return ok
}

// Valid 判断类目是否在固定七类里。
func (c Category) Valid() bool {
	_, ok := ParseCategory(string(c))
	return ok
}
