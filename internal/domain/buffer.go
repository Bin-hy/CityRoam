package domain

const (
	SameAreaBufferMinutes  = 10
	CrossAreaBufferMinutes = 20
)

func BufferMinutes(from, to POI) int {
	if from.Area != "" && from.Area == to.Area {
		return SameAreaBufferMinutes
	}
	return CrossAreaBufferMinutes
}
