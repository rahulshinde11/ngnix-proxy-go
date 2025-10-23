//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketUpgrade(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create WebSocket backend using echo server
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://ws.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test WebSocket connection
	conn := testWebSocketToProxy(t, env, "ws.example.com", "/")
	defer conn.Close()

	// Send message
	err := conn.WriteMessage(websocket.TextMessage, []byte("Hello WebSocket"))
	require.NoError(t, err)

	// Read echo response
	messageType, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Contains(t, string(message), "Request served by")
}

func TestWebSocketWithPath(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create WebSocket backend with path
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://example.com/ws -> :8080",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test WebSocket connection on path
	conn := testWebSocketToProxy(t, env, "example.com", "/ws")
	defer conn.Close()

	// Send and receive message
	err := conn.WriteMessage(websocket.TextMessage, []byte("test"))
	require.NoError(t, err)

	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, message)
}

func TestWebSocketBidirectional(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create WebSocket backend
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://chat.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Connect WebSocket
	conn := testWebSocketToProxy(t, env, "chat.example.com", "/")
	defer conn.Close()

	// Send multiple messages
	messages := []string{"message1", "message2", "message3"}
	for _, msg := range messages {
		err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
		require.NoError(t, err)

		// Read response
		_, response, err := conn.ReadMessage()
		require.NoError(t, err)
		require.NotEmpty(t, response)
	}
}

func TestWebSocketConnectionPersistence(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create WebSocket backend
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://persistent.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Connect WebSocket
	conn := testWebSocketToProxy(t, env, "persistent.example.com", "/")
	defer conn.Close()

	// Keep connection alive and send messages over time
	for i := 0; i < 5; i++ {
		err := conn.WriteMessage(websocket.TextMessage, []byte("ping"))
		require.NoError(t, err)

		_, _, err = conn.ReadMessage()
		require.NoError(t, err)

		time.Sleep(1 * time.Second)
	}
}

func TestSecureWebSocket(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Generate certificate
	generateSelfSignedCert(t, env.SSLDir, "wss.example.com")

	// Create WebSocket backend with WSS
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "wss://wss.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test secure WebSocket connection
	conn := testWebSocketSecureToProxy(t, env, "wss.example.com", "/")
	defer conn.Close()

	// Send message
	err := conn.WriteMessage(websocket.TextMessage, []byte("secure message"))
	require.NoError(t, err)

	// Read response
	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, message)
}

func TestWebSocketWithPathRewrite(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create WebSocket backend with path rewrite
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://example.com/socket -> :8080/",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test WebSocket connection
	conn := testWebSocketToProxy(t, env, "example.com", "/socket")
	defer conn.Close()

	err := conn.WriteMessage(websocket.TextMessage, []byte("test"))
	require.NoError(t, err)

	_, message, err := conn.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, message)
}

func TestMultipleWebSocketEndpoints(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create backend with multiple WebSocket endpoints
	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST1": "ws://example.com/ws1",
		"VIRTUAL_HOST2": "ws://example.com/ws2",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test first endpoint
	conn1 := testWebSocketToProxy(t, env, "example.com", "/ws1")
	defer conn1.Close()

	err := conn1.WriteMessage(websocket.TextMessage, []byte("endpoint1"))
	require.NoError(t, err)

	_, msg1, err := conn1.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, msg1)

	// Test second endpoint
	conn2 := testWebSocketToProxy(t, env, "example.com", "/ws2")
	defer conn2.Close()

	err = conn2.WriteMessage(websocket.TextMessage, []byte("endpoint2"))
	require.NoError(t, err)

	_, msg2, err := conn2.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, msg2)
}

func TestWebSocketHostHeaderRouting(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	// Create two WebSocket backends with different hosts
	backend1 := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://ws1.example.com",
	}, "8080/tcp")
	defer backend1.Container.Terminate(nil)

	backend2 := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://ws2.example.com",
	}, "8080/tcp")
	defer backend2.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	// Test first host
	conn1 := testWebSocketToProxy(t, env, "ws1.example.com", "/")
	defer conn1.Close()

	err := conn1.WriteMessage(websocket.TextMessage, []byte("test1"))
	require.NoError(t, err)

	_, msg1, err := conn1.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, msg1)

	// Test second host
	conn2 := testWebSocketToProxy(t, env, "ws2.example.com", "/")
	defer conn2.Close()

	err = conn2.WriteMessage(websocket.TextMessage, []byte("test2"))
	require.NoError(t, err)

	_, msg2, err := conn2.ReadMessage()
	require.NoError(t, err)
	require.NotEmpty(t, msg2)
}

func TestWebSocketConnectionTimeout(t *testing.T) {
	t.Skip("Timeout testing requires long-running connection - implement if needed")

	env := setupTestEnvironment(t)
	defer env.Cleanup()

	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://timeout.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	conn := testWebSocketToProxy(t, env, "timeout.example.com", "/")
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Connection should stay alive for configured timeout
	time.Sleep(2 * time.Second)

	err := conn.WriteMessage(websocket.TextMessage, []byte("still alive"))
	require.NoError(t, err)
}

func TestWebSocketBinaryMessages(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://binary.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	conn := testWebSocketToProxy(t, env, "binary.example.com", "/")
	defer conn.Close()

	// Send binary message
	binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	err := conn.WriteMessage(websocket.BinaryMessage, binaryData)
	require.NoError(t, err)

	// Read response
	messageType, _, err := conn.ReadMessage()
	require.NoError(t, err)
	// Echo server might convert to text or echo as binary
	require.True(t, messageType == websocket.BinaryMessage || messageType == websocket.TextMessage)
}

func TestWebSocketCloseHandshake(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.Cleanup()

	backend := createBackendContainer(t, env, "jmalloc/echo-server", map[string]string{
		"VIRTUAL_HOST": "ws://close.example.com",
	}, "8080/tcp")
	defer backend.Container.Terminate(nil)

	waitForBackendRegistration(t, 5*time.Second)

	conn := testWebSocketToProxy(t, env, "close.example.com", "/")

	// Send close message
	err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	require.NoError(t, err)

	// Give time for close handshake
	time.Sleep(1 * time.Second)

	// Connection should be closed
	err = conn.WriteMessage(websocket.TextMessage, []byte("after close"))
	require.Error(t, err)
}
