BASE_URL=$AI_TEST_DGC_URL
USERNAME=$AI_TEST_DGC_USR
PASSWORD=$AI_TEST_DGC_PWD

curl -sX POST "$BASE_URL/rest/aiCopilot/v1/tools/askGlossary" \
  -H "Content-Type: application/json" \
  -H "x-thread-id: f469bbfc-ee9b-4a3f-a393-a80d4e317bcb" \
  --user "$USERNAME:$PASSWORD" \
  -d '{
    "message": {
      "messagerRole": "user",
      "content": {
        "type": "text",
        "text": "Do we have a definition for chocolate?"
      },
      "context": {
        "originUrl": "'"$BASE_URL"'"
      }
    },
    "history": []
  }' | jq