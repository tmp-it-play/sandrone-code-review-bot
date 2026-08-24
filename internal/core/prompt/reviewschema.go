package prompt

func agenticReviewSchema(includeFileNotes bool) string {
	files := ""
	fileRule := ""
	overviewRule := "이번 요청의 <changed_files>에 실린 변경을 1~2문장으로 요약한다. 내부 배치나 검토 단위를 언급하지 않는다."
	if includeFileNotes {
		files = ",\n    \"files\": [{\"path\": \"파일 경로\", \"note\": \"이 파일에서 무엇이 바뀌었는지 한 문장\"}]"
		fileRule = "\n변경 파일이 8개 이하이므로 summary.files에 이번 요청의 모든 파일 경로와 비어 있지 않은 note를 정확히 한 번씩 포함한다."
		overviewRule = "변경 전체를 3~6문장으로 요약한다."
	}
	return `출력은 아래 JSON 하나만 낸다. 코드 블록 표시, 인사, 감탄, 설명을 JSON 밖에 덧붙이지 않는다.
JSON 키, severity 값, 파일 경로는 번역하거나 문체에 맞게 바꾸지 않는다.

{
  "summary": {
    "overview": "요약"` + files + `
  },
  "findings": [
    {
      "file": "파일 경로",
      "line": 12,
      "endLine": 0,
      "severity": "critical|major|minor|nit",
      "title": "한 줄 요약",
      "body": "무엇이 왜 문제인지와 근거",
      "suggestion": "line이 가리키는 줄을 그대로 대체할 코드 또는 빈 문자열",
      "evidence": "diff에서 + 기호를 제외하고 들여쓰기까지 정확히 복사한 추가·변경 줄 전체",
      "rootCause": "같은 원인의 모든 발생 위치에서 동일하게 쓸 짧고 구체적인 원인 식별문"
    }
  ]
}
summary.overview는 ` + overviewRule + fileRule + `
evidence는 <changed_files> 안의 diff에서 line부터 endLine까지 실제 추가된 연속 줄만 적는다. 가능한 가장 작은 범위를 쓰고, 한 줄로 충분하면 endLine은 0으로 둔다.
evidence에는 diff의 + 기호를 빼고 공백과 들여쓰기를 그대로 복사한다. 삭제 줄, 문맥 줄, current_content, read_file 결과나 diff 밖의 줄은 finding의 위치 근거로 쓰지 않는다.
evidence가 실제 추가 줄과 한 글자라도 다르면 finding을 만들지 않는다.
같은 rootCause의 여러 발생 위치는 각각 finding으로 내되 rootCause 문자열을 정확히 같게 유지한다.
변경을 끝까지 살펴본 뒤에도 남길 것이 정말 없을 때만 findings를 빈 배열로 둔다.`
}
