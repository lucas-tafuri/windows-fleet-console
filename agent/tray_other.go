//go:build !windows

package main

func serveTray(_ *Client) {
	waitSignal()
}
