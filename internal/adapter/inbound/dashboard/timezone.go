package dashboard

import "time"

var displayLocation = loadDisplayLocation()

func loadDisplayLocation() *time.Location {
	location, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		return time.FixedZone("KST", 9*60*60)
	}
	return location
}
