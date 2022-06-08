package valuehelpers

func Normalise[T ~float32 | ~float64](val, minVal, maxVal, newMin, newMax T) T {
	x := val - minVal
	y := (newMax - newMin) / (maxVal - minVal)
	return newMin + x*y
}

func Constrain[T ~float32 | ~float64](val, min, max T) T {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
