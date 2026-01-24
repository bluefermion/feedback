# Test Feedback Submission

Submit a test feedback entry to verify the system is working.

## Steps

1. Ensure the server is running:
   ```bash
   go run ./cmd/server
   ```

2. Submit test feedback:
   ```bash
   curl -X POST http://localhost:8080/api/feedback \
     -H "Content-Type: application/json" \
     -d '{
       "type": "Bug",
       "message": "Test feedback from CLI",
       "url": "http://localhost:8080/demo",
       "userAgent": "curl/test"
     }'
   ```

3. Verify submission:
   ```bash
   curl http://localhost:8080/api/feedback
   ```

4. Check admin view at http://localhost:8080/feedback
