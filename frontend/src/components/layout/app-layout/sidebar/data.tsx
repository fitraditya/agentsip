import {
  CallIcon,
  DatabaseIcon,
  GlobeIcon,
  LayoutGridIcon,
  PluginIcon,
} from "@/icons";
import type { NavItem } from "./types";

// Real product nav for agentsip.
export const navItems: NavItem[] = [
  {
    icon: <LayoutGridIcon className="size-6" />,
    key: "dashboard",
    path: "/",
  },
  {
    icon: <GlobeIcon className="size-6" />,
    key: "trunks",
    path: "/trunks",
  },
  {
    icon: <PluginIcon className="size-6" />,
    key: "providers",
    subItems: [
      { key: "sttProviders", path: "/stt-providers" },
      { key: "ttsProviders", path: "/tts-providers" },
      { key: "llmProviders", path: "/llm-providers" },
    ],
  },
  {
    icon: <DatabaseIcon className="size-6" />,
    key: "knowledgeBases",
    path: "/knowledge-bases",
  },
  {
    icon: <CallIcon className="size-6" />,
    key: "calls",
    path: "/calls",
  },
];

export const othersItems: NavItem[] = [];

export const supportItems: NavItem[] = [];
