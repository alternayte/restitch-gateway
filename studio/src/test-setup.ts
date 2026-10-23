import "@testing-library/jest-dom/vitest";
import "@/api-client";
import { client } from "@/api/client.gen";

// Node's Request needs an absolute URL; a browser resolves the relative one.
client.setConfig({ baseUrl: "http://localhost:3000" });
