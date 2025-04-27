import React, { useState, useEffect, useCallback } from "react";

const MachineDataComponent = () => {
  const [data, setData] = useState(null);
  const [error, setError] = useState(null);
  const [isConnected, setIsConnected] = useState(false);
  const [eventSource, setEventSource] = useState(null);

  // Function to connect to SSE
  const connectSSE = useCallback(() => {
    const source = new EventSource(
      "http://localhost:3000/api/sse/machine-data"
    );

    source.onopen = () => {
      console.log("SSE Connection opened");
      setIsConnected(true);
    };

    source.addEventListener("connected", (event) => {
      console.log("Connected to SSE server");
      const data = JSON.parse(event.data);
      console.log("Connection status:", data.status);
    });

    source.addEventListener("update", (event) => {
      const newData = JSON.parse(event.data);
      console.log("New data received:", newData);
      setData(newData);
      setError(null);
    });

    source.addEventListener("error", (event) => {
      const errorData = JSON.parse(event.data);
      console.error("SSE Error:", errorData);
      setError(errorData);
      source.close();
      setIsConnected(false);
    });

    setEventSource(source);
  }, []);

  // Function to disconnect from SSE
  const disconnectSSE = useCallback(() => {
    if (eventSource) {
      eventSource.close();
      console.log("SSE Connection closed");
      setIsConnected(false);
      setEventSource(null);
    }
  }, [eventSource]);

  // Initial data fetch
  useEffect(() => {
    const fetchInitialData = async () => {
      try {
        const response = await fetch(
          "http://localhost:3000/api/sse/current-data"
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
      if (eventSource) {
        eventSource.close();
      }
    };
  }, [eventSource]);

  return (
    <div className="machine-data-container">
      <h2>Machine Data Monitor</h2>

      <div className="connection-controls">
        <button
          onClick={connectSSE}
          disabled={isConnected}
          className="connect-btn"
        >
          Connect
        </button>
        <button
          onClick={disconnectSSE}
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

export default MachineDataComponent;
