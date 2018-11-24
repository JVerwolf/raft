package raft

import (
    "github.com/google/go-cmp/cmp"
)

type NodeType int

const (
    Leader    NodeType = iota
    Follower
    Candidate
)

type Node struct {
    nodeType NodeType

    // PERSISTENT STATE:

    // Latest term server has seen (initialized to 0
    // on first boot, increases monotonically).
    currentTerm int

    // CandidateId that received vote in current
    // term (or null if none).
    votedFor int

    // Log entries; each entry contains command
    // for state machine, and term when entry
    // was received by leader (first index is 1).
    log []Entry

    // VOLATILE STATE ON ALL SERVERS:

    // Index of highest log entry known to be
    // committed (initialized to 0, increases
    // monotonically).
    commitIndex int

    // Index of highest log entry applied to state
    // machine (initialized to 0, increases
    // monotonically).
    lastApplied int

    // VOLATILE STATE ON LEADERS

    //  For each server, index of the next log entry
    //  to send to that server (initialized to leader
    //  last log index + 1.
    nextIndex []int

    // For each server, index of highest log entry
    // known to be replicated on server
    // (initialized to 0, increases monotonically).
    matchIndex []int
}

type Entry struct {
    Command string
    Index   int
    TermNum int
}
