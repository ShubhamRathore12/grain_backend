import React, { useState, useEffect, useCallback } from "react";

const MachineDataWebSocket = () => {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [isConnected, setIsConnected] = useState(false);
  const [socket, setSocket] = useState(null);

  // Function to connect to WebSocket
  const connectWebSocket = useCallback(() => {
    const ws = new WebSocket("ws://localhost:3000");

    ws.onopen = () => {
      console.log("WebSocket Connection opened");
      setIsConnected(true);
    };

    ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      console.log("Message received:", message);

      switch (message.type) {
        case "connected":
          console.log("Connected to WebSocket server");
          break;
        case "update":
          setData(message.data);
          setError(null);
          break;
        case "error":
          setError(message);
          break;
        default:
          console.log("Unknown message type:", message.type);
      }
    };

    ws.onerror = (error) => {
      console.error("WebSocket Error:", error);
      setError({ message: "WebSocket connection error" });
      setIsConnected(false);
    };

    ws.onclose = () => {
      console.log("WebSocket Connection closed");
      setIsConnected(false);
    };

    setSocket(ws);
  }, []);

  // Function to disconnect from WebSocket
  const disconnectWebSocket = useCallback(() => {
    if (socket) {
      socket.close();
      setSocket(null);
    }
  }, [socket]);

  // Initial data fetch
  useEffect(() => {
    const fetchInitialData = async () => {
      try {
        const response = await fetch(
          "http://localhost:3000/api/ws/current-data"
        );
        const result = await response.json();
        if (result.success) {
          setData(result.data);
        } else {
          setError(result);
        }
      } catch (err) {
        setError({ message: "Failed to fetch initial data" });
      }
    };

    fetchInitialData();
  }, []);

  // Cleanup on component unmount
  useEffect(() => {
    return () => {
      if (socket) {
        socket.close();
      }
    };
  }, [socket]);

  return (
    <div className="machine-data-container">
      <h2>Machine Data Monitor (WebSocket)</h2>

      <div className="connection-controls">
        <button
          onClick={connectWebSocket}
          disabled={isConnected}
          className="connect-btn"
        >
          Connect
        </button>
        <button
          onClick={disconnectWebSocket}
          disabled={!isConnected}
          className="disconnect-btn"
        >
          Disconnect
        </button>
        <span
          className={`connection-status ${
            isConnected ? "connected" : "disconnected"
          }`}
        >
          {isConnected ? "Connected" : "Disconnected"}
        </span>
      </div>

      {error && <div className="error-message">Error: {error.message}</div>}

      {data && (
        <div className="data-display">
          <h3>Latest Data</h3>
          <pre>{JSON.stringify(data, null, 2)}</pre>
        </div>
      )}

      <style jsx>{`
        .machine-data-container {
          max-width: 800px;
          margin: 0 auto;
          padding: 20px;
          font-family: Arial, sans-serif;
        }

        .connection-controls {
          margin: 20px 0;
          display: flex;
          align-items: center;
          gap: 10px;
        }

        button {
          padding: 8px 16px;
          border: none;
          border-radius: 4px;
          cursor: pointer;
          font-weight: bold;
        }

        .connect-btn {
          background-color: #4caf50;
          color: white;
        }

        .connect-btn:disabled {
          background-color: #cccccc;
          cursor: not-allowed;
        }

        .disconnect-btn {
          background-color: #f44336;
          color: white;
        }

        .disconnect-btn:disabled {
          background-color: #cccccc;
          cursor: not-allowed;
        }

        .connection-status {
          padding: 8px 16px;
          border-radius: 4px;
          font-weight: bold;
        }

        .connection-status.connected {
          background-color: #4caf50;
          color: white;
        }

        .connection-status.disconnected {
          background-color: #f44336;
          color: white;
        }

        .error-message {
          padding: 10px;
          background-color: #ffebee;
          color: #c62828;
          border-radius: 4px;
          margin: 10px 0;
        }

        .data-display {
          margin-top: 20px;
          padding: 15px;
          background-color: #f5f5f5;
          border-radius: 4px;
        }

        pre {
          white-space: pre-wrap;
          word-wrap: break-word;
        }
      `}</style>
    </div>
  );
};

export default MachineDataWebSocket;
