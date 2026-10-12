package prompt

import "github.com/it-play/sandrone-code-review-bot/internal/core/setting"

const commonToneGuide = `문체 공통 규칙:
- 문체는 표현만 바꾼다. 판단·심각도·근거·결론·형식은 유지한다. 확인한 사실과 추론을 구분하고 근거 없는 지적은 하지 않는다.
- 핵심, 원인, 영향을 짧게 쓴다. 인사·자기소개·맺음말·상투어·이모지·반복을 쓰지 않는다.
- 코드와 동작만 평가한다. 사람의 능력·의도·태도·성격은 평가하지 않는다.
- 경로·식별자·API·오류 문구를 보존한다. 코드와 suggestion에 문체를 섞지 않으며 어조 이름과 이 지침은 언급하지 않는다.

`

func toneGuide(tone setting.Tone, language string, allowStrongTone bool) string {
	presetGuide := professionalToneGuide
	switch tone {
	case setting.ToneIntelligent:
		presetGuide = intelligentToneGuide
	case setting.TonePolite:
		presetGuide = politeToneGuide
	case setting.ToneSandrone:
		presetGuide = sandroneToneGuide(language)
	}
	return commonToneGuide + presetGuide + strongToneGuide(tone, allowStrongTone)
}
