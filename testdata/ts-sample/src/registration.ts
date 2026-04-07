// Used by main.ts but with deliberately unused enum members and class
// members so we can verify how knip serializes them.

export enum Status {
  Active = "active",
  Inactive = "inactive",
  Obsolete = "obsolete", // never referenced
}

export class Registry {
  private items: string[] = [];

  add(item: string): void {
    this.items.push(item);
  }

  // Never called from anywhere — knip should flag this.
  remove(item: string): void {
    this.items = this.items.filter((i) => i !== item);
  }
}
