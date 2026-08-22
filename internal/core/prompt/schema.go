package prompt

const reviewSchema = `출력은 아래 JSON 하나만 낸다. 코드 블록 표시나 설명을 덧붙이지 않는다.

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
      "suggestion": "고친 코드 조각 또는 빈 문자열"
    }
  ]
}

지적할 것이 없으면 findings를 빈 배열로 둔다.`

const summarySchema = `출력은 아래 JSON 하나만 낸다. 코드 블록 표시나 설명을 덧붙이지 않는다.

{
  "summary": {
    "overview": "변경 전체를 3~6문장으로 요약",
    "files": [{"path": "파일 경로", "note": "이 파일에서 무엇이 바뀌었는지 한 문장"}]
  }
}`
