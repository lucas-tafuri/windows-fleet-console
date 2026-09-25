//go:build windows

package main

import "testing"

func TestBusiestGPU(t *testing.T) {
	rows := []gpuEngine{
		{"pid_1_luid_a_phys_0_eng_0_engtype_3D", 30},
		{"pid_2_luid_a_phys_0_eng_0_engtype_3D", 40},
		{"pid_1_luid_a_phys_0_eng_1_engtype_Copy", 50},
		{"pid_1_luid_b_phys_0_eng_0_engtype_3D", 60},
	}
	if value := busiestGPU(rows); value == nil || *value != 70 {
		t.Fatalf("want busiest shared engine 70, got %v", value)
	}
	if busiestGPU(nil) != nil {
		t.Fatal("missing counters must be unavailable")
	}
	if value := busiestGPU([]gpuEngine{{"pid_1_luid_a", 0}}); value == nil || *value != 0 {
		t.Fatal("idle counter must be zero")
	}
	if value := busiestGPU([]gpuEngine{{"pid_1_luid_a", 120}}); value == nil || *value != 100 {
		t.Fatal("counter must clamp to 100")
	}
}
