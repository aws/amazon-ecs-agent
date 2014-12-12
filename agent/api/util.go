package api

// RemoveFromTaskArray removes the element at ndx from an array of task
// pointers, arr. If the ndx is out of bounds, it returns arr unchanged.
func RemoveFromTaskArray(arr []*Task, ndx int) []*Task {
	if ndx < 0 || ndx >= len(arr) {
		return arr
	}
	return append(arr[0:ndx], arr[ndx+1:len(arr)]...)
}
