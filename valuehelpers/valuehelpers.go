package valuehelpers

import "golang.org/x/exp/constraints"

/*
Normalise a value from one range (minVal:maxVal) to fit within a new range (newMin:newMax)
*/
func Normalise[T constraints.Integer | constraints.Float](val, minVal, maxVal, newMin, newMax T) T {
	x := val - minVal
	newRange := newMax - newMin
	oldRange := maxVal - minVal
	if newRange > oldRange {
		y := newRange / oldRange
		return newMin + x*y
	} else {
		y := oldRange / newRange
		return newMin + x/y
	}
}

/*
Constrain a value between a lower (min) and upper (max) limit
*/
func Constrain[T constraints.Integer | constraints.Float](val, min, max T) T {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
