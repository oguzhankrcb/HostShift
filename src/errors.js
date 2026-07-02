export class SafetyError extends Error {
  constructor(message) {
    super(message);
    this.name = "SafetyError";
  }
}

export class ValidationError extends Error {
  constructor(message) {
    super(message);
    this.name = "ValidationError";
  }
}
