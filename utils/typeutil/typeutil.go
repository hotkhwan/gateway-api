// utils/typeutil/typeutil.go
package typeutil

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func Str(v any) string {
    switch x := v.(type) {
    case string:
        return x
    case []byte:
        return string(x)
    case fmt.Stringer:
        return x.String()
    default:
        return ""
    }
}

func Int(v any) int {
    switch x := v.(type) {
    case int:
        return x
    case int32:
        return int(x)
    case int64:
        return int(x)
    case float64:
        return int(x)
    default:
        return 0
    }
}

func Int64(v any) int64 {
    switch x := v.(type) {
    case int64:
        return x
    case int32:
        return int64(x)
    case int:
        return int64(x)
    case float64:
        return int64(x)
    default:
        return 0
    }
}

func Time(v any) time.Time {
    switch x := v.(type) {
    case time.Time:
        return x
    case primitive.DateTime:
        return x.Time()
    default:
        return time.Time{}
    }
}

func ObjectIdHex(v any) string {
    switch x := v.(type) {
    case primitive.ObjectID:
        return x.Hex()
    default:
        return ""
    }
}
