const words = ["No", "One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight", "Nine", "Ten", "Eleven", "Twelve",
  "Thirteen", "Fourteen", "Fifteen", "Sixteen", "Seventeen", "Eighteen", "Nineteen", "Twenty"];

/** A count as the headlines write it: a word up to twenty, digits after. */
export function countWord(count: number, lower = false): string {
  const word = count >= 0 && count <= 20 ? words[count]! : String(count);
  return lower ? word.toLowerCase() : word;
}

/** The singular or the plural for a count. */
export function plural(count: number, one: string, many = `${one}s`): string {
  return count === 1 ? one : many;
}
