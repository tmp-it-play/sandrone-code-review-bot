package reviewworkflow

import "fmt"

const MaxUnitOrderKeyLength = 160

func RootUnitOrderKey(ordinal int) string {
	return fmt.Sprintf("%08d", ordinal)
}

func ChildUnitOrderKey(parent string, index int) string {
	return fmt.Sprintf("%s.%d", parent, index)
}
