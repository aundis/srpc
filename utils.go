package srpc

import "github.com/gogf/gf/v2/frame/g"

func BasicValue(v interface{}) interface{} {
	return g.Map{
		"value": v,
	}
}
