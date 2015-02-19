package ttime

import (
	"testing"
	"time"
)

func TestWarp(t *testing.T) {
	testTime := NewTestTime()
	SetTime(testTime)

	now := Now()
	testTime.Warp(5 * time.Minute)
	if Now().Sub(now) < 5*time.Minute {
		t.Error("Expected to warp forwards 5 minutes")
	}
}

func TestLudicrousSpeed(t *testing.T) {
	testTime := NewTestTime()
	SetTime(testTime)
	testTime.LudicrousSpeed(true)

	realnow := time.Now()
	now := Now()
	Sleep(5 * time.Minute)
	if Now().Sub(now) < 5*time.Minute {
		t.Error("Expected to skip forwards 5 minutes")
	}
	if time.Since(realnow) > 1*time.Second {
		t.Error("It shouldn't have really taken 5 minutes though")
	}
	if Since(realnow) < 5*time.Minute {
		t.Error("Unless you compare using ttime")
	}
}

func TestSleepWarp(t *testing.T) {
	testTime := NewTestTime()
	SetTime(testTime)

	done := make(chan bool)

	realnow := time.Now()
	go func() {
		Sleep(1 * time.Second)
		Sleep(1 * time.Second)
		done <- true
	}()
	testTime.Warp(15 * time.Second)
	<-done
	if time.Since(realnow) > 1*time.Second {
		t.Error("Time should have been warped")
	}
}
