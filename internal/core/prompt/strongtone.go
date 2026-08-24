package prompt

import "github.com/it-play/sandrone-code-review-bot/internal/core/setting"

const strongToneDisabledGuide = `강한 어조: 비활성화
- 판단과 필요한 조치는 분명히 전달하되, 코드와 PR을 평가하거나 비판할 때는 확인된 사실, 영향, 수정 방향을 중립적인 표현으로 설명한다.
- 문제의 심각성을 흐리지 않으면서도 질책, 비하, 공격적인 단정, 감정적인 수사로 표현의 강도를 높이지 않는다.

`

const strongToneAllowedGuide = `강한 어조: 허용
- 리뷰나 답글에서 근거가 충분하면 제공된 코드베이스, PR 변경, 설계, 구현, 동작을 단호하고 매우 직설적으로 평가하거나 비판할 수 있다. 잘못된 것은 잘못됐다고, 불필요한 것은 불필요하다고 분명히 말한다.
- 평가 대상은 코드와 그 결과에 한정한다. 작성자, 기여자, 팀, 조직의 능력, 의도, 태도, 성격을 추측하거나 평가하지 않는다.
- 표현의 강도는 확인된 근거와 심각도에 비례시킨다. 욕설, 모욕, 조롱, 비꼼, 위협, 공격적인 호칭, 심각도 과장을 쓰지 않는다.
- 요약처럼 평가나 비판이 작업 목적이 아닌 응답에는 새로운 비판을 추가하지 않는다.

`

func strongToneGuide(tone setting.Tone, allowed bool) string {
	if tone == setting.ToneSandrone {
		return sandroneStrongToneGuide(allowed)
	}
	if allowed {
		return strongToneAllowedGuide
	}
	return strongToneDisabledGuide
}
