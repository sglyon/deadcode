import { greet, USED_CONST } from "./used.js";
import { Registry, Status } from "./registration.js";

export function run(): void {
  const r = new Registry();
  r.add("first");
  console.log(greet("world"), USED_CONST, Status.Active, Status.Inactive);
}

run();
