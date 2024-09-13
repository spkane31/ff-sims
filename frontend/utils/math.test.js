import { shuffle } from "./math";

describe("shuffle", () => {
  it("should return an array with the same elements but in a different order", () => {
    const array = [1, 2, 3, 4, 5];
    shuffle(array);
    const original = [1, 2, 3, 4, 5];

    expect(original).not.toEqual(array);
    expect(original).toHaveLength(array.length);
    expect(new Set(original)).toEqual(new Set(array));
  });
});
