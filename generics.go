package generics

func GetValue[T any](v *T) T {
	if v == nil {
		return *new(T)
	}
	return *v
}
