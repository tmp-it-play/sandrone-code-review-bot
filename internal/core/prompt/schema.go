package prompt

const reviewSchema = `출력은 아래 JSON 하나만 낸다. 코드 블록 표시, 인사, 감탄, 설명을 JSON 밖에 덧붙이지 않는다.
JSON 키, severity 값, 파일 경로는 번역하거나 문체에 맞게 바꾸지 않는다.

{
  "summary": {
    "overview": "변경 전체를 3~6문장으로 요약",
    "files": [{"path": "파일 경로", "note": "이 파일에서 무엇이 바뀌었는지 한 문장"}]
  },
  "findings": [
    {
      "file": "파일 경로",
      "line": 12,
      "endLine": 0,
      "severity": "critical|major|minor|nit",
      "title": "한 줄 요약",
      "body": "무엇이 왜 문제인지와 근거",
      "suggestion": "line이 가리키는 줄을 그대로 대체할 코드 또는 빈 문자열"
    }
  ]
}

변경을 끝까지 살펴본 뒤에도 남길 것이 정말 없을 때만 findings를 빈 배열로 둔다.`

const summarySchema = `출력은 아래 JSON 하나만 낸다. 코드 블록 표시, 인사, 감탄, 설명을 JSON 밖에 덧붙이지 않는다.
JSON 키와 파일 경로는 번역하거나 문체에 맞게 바꾸지 않는다.

{
  "summary": {
    "overview": "변경 전체를 3~6문장으로 요약",
    "files": [{"path": "파일 경로", "note": "이 파일에서 무엇이 바뀌었는지 한 문장"}]
  }
}`

const suggestionGuide = `suggestion 사용 규칙:
- suggestion은 GitHub의 제안(Suggested change)으로 그대로 렌더링된다. 받는 사람이 버튼 한 번으로 커밋한다.
- line이 가리키는 그 줄을 통째로 대체하는 완성된 코드만 넣는다. 원본 들여쓰기를 그대로 유지한다.
- 한 줄을 여러 줄로 늘리는 것은 괜찮다. 다만 결과가 그 줄의 자리에 그대로 들어가 컴파일되는 코드여야 한다.
- 설명, 주변 문맥, 생략 표시(...), diff 기호(+/-)를 넣지 않는다.
- 여러 곳을 함께 고쳐야 하거나 어디를 바꿀지 특정할 수 없으면 빈 문자열로 두고 body로만 설명한다.
- 확신이 서지 않으면 빈 문자열로 둔다. 잘못된 제안은 지적하지 않느니만 못하다.

`
