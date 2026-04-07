export function greet(name: string): string {
  return `hello, ${name}`;
}

export const USED_CONST = 42;

// Knip should flag this — exported but no consumer.
export function unusedHelper(x: number): number {
  return x * 2;
}

// Knip should flag this — unused type.
export type UnusedShape = {
  id: string;
  name: string;
};
