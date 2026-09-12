import type { Room, Invitation, Request } from "../../../shared/model";
export type Dialog =
  | { type: "create" }
  | { type: "invite"; room: Room; invitation: Invitation }
  | {
      type: "confirm";
      title: string;
      description: string;
      request: Request;
      exit?: boolean;
    };
