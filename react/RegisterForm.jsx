import React, { useState } from "react";

const RegisterForm = () => {
  const [formData, setFormData] = useState({
    accountType: "",
    firstName: "",
    lastName: "",
    username: "",
    email: "",
    phoneNumber: "",
    company: "",
    password: "",
    monitorAccess: [],
  });
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(false);
  const [loading, setLoading] = useState(false);

  const handleChange = (e) => {
    const { name, value, type } = e.target;
    if (type === "checkbox") {
      setFormData((prev) => ({
        ...prev,
        monitorAccess: e.target.checked
          ? [...prev.monitorAccess, value]
          : prev.monitorAccess.filter((item) => item !== value),
      }));
    } else {
      setFormData((prev) => ({
        ...prev,
        [name]: value,
      }));
    }
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError(null);
    setSuccess(false);

    try {
      const response = await fetch("http://localhost:3000/api/register", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(formData),
      });

      const data = await response.json();

      if (!response.ok) {
        throw new Error(data.message || "Registration failed");
      }

      setSuccess(true);
      setFormData({
        accountType: "",
        firstName: "",
        lastName: "",
        username: "",
        email: "",
        phoneNumber: "",
        company: "",
        password: "",
        monitorAccess: [],
      });
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="register-container">
      <h2>User Registration</h2>

      {error && <div className="error-message">{error}</div>}

      {success && (
        <div className="success-message">
          Registration successful! You can now login.
        </div>
      )}

      <form onSubmit={handleSubmit} className="register-form">
        <div className="form-group">
          <label>Account Type</label>
          <select
            name="accountType"
            value={formData.accountType}
            onChange={handleChange}
            required
          >
            <option value="">Select Account Type</option>
            <option value="admin">Admin</option>
            <option value="user">User</option>
          </select>
        </div>

        <div className="form-group">
          <label>First Name</label>
          <input
            type="text"
            name="firstName"
            value={formData.firstName}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Last Name</label>
          <input
            type="text"
            name="lastName"
            value={formData.lastName}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Username</label>
          <input
            type="text"
            name="username"
            value={formData.username}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Email</label>
          <input
            type="email"
            name="email"
            value={formData.email}
            onChange={handleChange}
          />
        </div>

        <div className="form-group">
          <label>Phone Number</label>
          <input
            type="tel"
            name="phoneNumber"
            value={formData.phoneNumber}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Company</label>
          <input
            type="text"
            name="company"
            value={formData.company}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Password</label>
          <input
            type="password"
            name="password"
            value={formData.password}
            onChange={handleChange}
            required
          />
        </div>

        <div className="form-group">
          <label>Monitor Access</label>
          <div className="checkbox-group">
            <label>
              <input
                type="checkbox"
                value="machine1"
                checked={formData.monitorAccess.includes("machine1")}
                onChange={handleChange}
              />
              Machine 1
            </label>
            <label>
              <input
                type="checkbox"
                value="machine2"
                checked={formData.monitorAccess.includes("machine2")}
                onChange={handleChange}
              />
              Machine 2
            </label>
            <label>
              <input
                type="checkbox"
                value="machine3"
                checked={formData.monitorAccess.includes("machine3")}
                onChange={handleChange}
              />
              Machine 3
            </label>
          </div>
        </div>

        <button type="submit" disabled={loading} className="submit-btn">
          {loading ? "Registering..." : "Register"}
        </button>
      </form>

      <style jsx>{`
        .register-container {
          max-width: 600px;
          margin: 0 auto;
          padding: 20px;
          font-family: Arial, sans-serif;
        }

        .register-form {
          display: flex;
          flex-direction: column;
          gap: 15px;
        }

        .form-group {
          display: flex;
          flex-direction: column;
          gap: 5px;
        }

        label {
          font-weight: bold;
        }

        input,
        select {
          padding: 8px;
          border: 1px solid #ddd;
          border-radius: 4px;
          font-size: 16px;
        }

        .checkbox-group {
          display: flex;
          flex-direction: column;
          gap: 5px;
        }

        .checkbox-group label {
          display: flex;
          align-items: center;
          gap: 5px;
          font-weight: normal;
        }

        .submit-btn {
          padding: 10px 20px;
          background-color: #4caf50;
          color: white;
          border: none;
          border-radius: 4px;
          cursor: pointer;
          font-size: 16px;
          margin-top: 10px;
        }

        .submit-btn:disabled {
          background-color: #cccccc;
          cursor: not-allowed;
        }

        .error-message {
          padding: 10px;
          background-color: #ffebee;
          color: #c62828;
          border-radius: 4px;
          margin-bottom: 20px;
        }

        .success-message {
          padding: 10px;
          background-color: #e8f5e9;
          color: #2e7d32;
          border-radius: 4px;
          margin-bottom: 20px;
        }
      `}</style>
    </div>
  );
};

export default RegisterForm;
