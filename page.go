package gorm_max

// Page 分页参数,Current 从 1 开始。
type Page struct {
	Current int64
	Size    int64
}

func (p *Page) offset() int64 {
	cur := p.Current
	if cur < 1 {
		cur = 1
	}
	return (cur - 1) * p.Size
}

// PageResult 分页结果。
type PageResult[T any] struct {
	Records []*T
	Total   int64
	Current int64
	Size    int64
	Pages   int64
}
