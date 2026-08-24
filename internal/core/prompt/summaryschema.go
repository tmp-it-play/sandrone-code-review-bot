package prompt

const summarySchema = `출력은 아래 JSON 하나만 낸다. 코드 블록 표시, 인사, 감탄, 설명을 JSON 밖에 덧붙이지 않는다.
JSON 키와 파일 경로는 번역하거나 문체에 맞게 바꾸지 않는다.

{
  "summary": {
    "overview": "변경 전체를 3~6문장으로 요약",
    "files": [{"path": "파일 경로", "note": "이 파일에서 무엇이 바뀌었는지 한 문장"}]
  }
}`
