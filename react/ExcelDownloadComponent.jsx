import React, { useState, useEffect } from "react";

const ExcelDownloadComponent = () => {
  const [tables, setTables] = useState([]);
  const [selectedTable, setSelectedTable] = useState("");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [downloadAll, setDownloadAll] = useState(true);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  // Fetch available tables on component mount
  useEffect(() => {
    fetchTables();
  }, []);

  const fetchTables = async () => {
    try {
      const response = await fetch("/api/excel/tables");
      const data = await response.json();
      if (data.success) {
        setTables(data.tables);
        if (data.tables.length > 0) {
          setSelectedTable(data.tables[0]);
        }
      }
    } catch (err) {
      setError("Failed to fetch available tables");
      console.error("Error fetching tables:", err);
    }
  };

  const handleDownload = async () => {
    if (!selectedTable) {
      setError("Please select a table");
      return;
    }

    setLoading(true);
    setError("");

    try {
      // Build query parameters
      const params = new URLSearchParams({
        table: selectedTable,
        downloadAll: downloadAll ? "true" : "false",
      });

      if (fromDate) params.append("fromDate", fromDate);
      if (toDate) params.append("toDate", toDate);

      const response = await fetch(
        `/api/excel/download-excel?${params.toString()}`
      );

      if (!response.ok) {
        let errorData = {};
        try {
          errorData = await response.json();
        } catch (e) {}
        if (errorData && errorData.downloadUrl) {
          window.location.href = errorData.downloadUrl;
          return;
        }
        throw new Error(errorData.error || "Download failed");
      }

      // Create blob and download
      const blob = await response.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${selectedTable}_data_${new Date()
        .toISOString()
        .replace(/[:.]/g, "-")}.xlsx`;
      document.body.appendChild(a);
      a.click();
      window.URL.revokeObjectURL(url);
      document.body.removeChild(a);
    } catch (err) {
      setError(err.message || "Download failed");
      console.error("Download error:", err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        maxWidth: "600px",
        margin: "20px auto",
        padding: "20px",
        border: "1px solid #ddd",
        borderRadius: "8px",
        backgroundColor: "#f9f9f9",
      }}
    >
      <h2 style={{ textAlign: "center", color: "#333", marginBottom: "20px" }}>
        Download Excel Data
      </h2>

      {error && (
        <div
          style={{
            padding: "10px",
            marginBottom: "15px",
            backgroundColor: "#ffebee",
            color: "#c62828",
            borderRadius: "4px",
            border: "1px solid #ffcdd2",
          }}
        >
          {error}
        </div>
      )}

      <div style={{ marginBottom: "15px" }}>
        <label
          style={{ display: "block", marginBottom: "5px", fontWeight: "bold" }}
        >
          Select Table:
        </label>
        <select
          value={selectedTable}
          onChange={(e) => setSelectedTable(e.target.value)}
          style={{
            width: "100%",
            padding: "8px",
            border: "1px solid #ddd",
            borderRadius: "4px",
          }}
        >
          <option value="">Select a table...</option>
          {tables.map((table) => (
            <option key={table} value={table}>
              {table}
            </option>
          ))}
        </select>
      </div>

      <div style={{ marginBottom: "15px" }}>
        <label
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "10px",
            fontWeight: "bold",
          }}
        >
          <input
            type="checkbox"
            checked={downloadAll}
            onChange={(e) => setDownloadAll(e.target.checked)}
          />
          Download all (may be limited by server)
        </label>
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "1fr 1fr",
          gap: "15px",
          marginBottom: "20px",
        }}
      >
        <div>
          <label
            style={{
              display: "block",
              marginBottom: "5px",
              fontWeight: "bold",
            }}
          >
            From Date (optional):
          </label>
          <input
            type="date"
            value={fromDate}
            onChange={(e) => setFromDate(e.target.value)}
            style={{
              width: "100%",
              padding: "8px",
              border: "1px solid #ddd",
              borderRadius: "4px",
            }}
          />
        </div>
        <div>
          <label
            style={{
              display: "block",
              marginBottom: "5px",
              fontWeight: "bold",
            }}
          >
            To Date (optional):
          </label>
          <input
            type="date"
            value={toDate}
            onChange={(e) => setToDate(e.target.value)}
            style={{
              width: "100%",
              padding: "8px",
              border: "1px solid #ddd",
              borderRadius: "4px",
            }}
          />
        </div>
      </div>

      <button
        onClick={handleDownload}
        disabled={loading || !selectedTable}
        style={{
          width: "100%",
          padding: "12px",
          backgroundColor: loading ? "#ccc" : "#4CAF50",
          color: "white",
          border: "none",
          borderRadius: "4px",
          cursor: loading ? "not-allowed" : "pointer",
          fontSize: "16px",
          fontWeight: "bold",
        }}
      >
        {loading ? "Downloading..." : "Download Excel File"}
      </button>

      <div
        style={{
          marginTop: "15px",
          padding: "10px",
          backgroundColor: "#e3f2fd",
          borderRadius: "4px",
          fontSize: "14px",
          color: "#1976d2",
        }}
      >
        <strong>Instructions:</strong>
        <ul style={{ margin: "5px 0", paddingLeft: "20px" }}>
          <li>Select a table from the dropdown</li>
          <li>Set the limit (max 10,000 rows)</li>
          <li>Set the offset for pagination</li>
          <li>Optionally set date range filters</li>
          <li>Click download to get the Excel file</li>
        </ul>
      </div>
    </div>
  );
};

export default ExcelDownloadComponent;
