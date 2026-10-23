import React from "react";
import { createRoot } from "react-dom/client";
import { Shell } from "@/components/shell";
import { FleetView } from "@/components/fleet-view";
import SoftwarePage from "@/app/software/page";
import JobsPage from "@/app/jobs/page";
import EnrollPage from "@/app/enroll/page";
import "@/app/globals.css";

const route = window.location.pathname.replace(/\/$/, "") || "/";
const Page = route === "/software" ? SoftwarePage : route === "/jobs" ? JobsPage : route === "/enroll" ? EnrollPage : FleetView;
createRoot(document.getElementById("root")!).render(<React.StrictMode><Shell><Page /></Shell></React.StrictMode>);
