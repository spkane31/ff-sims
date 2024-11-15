module.exports = {
  preset: "ts-jest",
  testEnvironment: "node",
  moduleFileExtensions: ["ts", "tsx", "js", "jsx", "json", "node"],
  transform: {
    "^.+\\.tsx?$": "ts-jest",
    "^.+\\.jsx?$": "babel-jest",
  },
  testMatch: ["**/tests/**/*.test.ts", "**/tests/**/*.test.js"],
  globals: {
    "ts-jest": {
      tsconfig: "tsconfig.json",
    },
  },
};
