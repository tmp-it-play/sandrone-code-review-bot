package backoff

import "time"

type Policy struct {
	Initial time.Duration
	Maximum time.Duration
}

func (p Policy) Delay(attempt int, seed uint64) time.Duration {
	if p.Initial <= 0 || p.Maximum <= 0 {
		return 0
	}
	maximum := p.Maximum
	if attempt < 0 {
		attempt = 0
	}
	ceiling := min(p.Initial, maximum)
	for range attempt {
		if ceiling >= maximum || ceiling > maximum/2 {
			ceiling = maximum
			break
		}
		ceiling *= 2
	}
	if ceiling > maximum {
		ceiling = maximum
	}
	spread := ceiling / 5
	if spread <= 0 {
		return ceiling
	}
	floor := ceiling - spread
	mixed := mix(seed ^ uint64(attempt+1)*0x9e3779b97f4a7c15)
	return floor + time.Duration(mixed%uint64(spread+1))
}

func mix(value uint64) uint64 {
	value ^= value >> 30
	value *= 0xbf58476d1ce4e5b9
	value ^= value >> 27
	value *= 0x94d049bb133111eb
	return value ^ (value >> 31)
}
