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
    peers    []*Node

    // The following values are from the states
    // described in the raft paper:

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
func NewNode(peers []*Node) (this *Node) {
    this = new(Node)
    this.nodeType = Follower

    // Initialize (non-leader)State described in the Raft paper:
    this.currentTerm = 0
    this.votedFor = -1
    this.log = make([]Entry, 0) // TODO: Initialize to 1?
    this.commitIndex = 0
    this.lastApplied = 0

    // Distribute knowledge to peers.
    // In a real-world scenario, this would be handled by a
    // configuration manager, such as Zookeeper.
    peers = append(peers, this)
    for _, node := range peers {
        node.peers = peers
    }
    return
}

type Entry struct {
    Command string
    Index   int
    TermNum int
}

func (this *Node) AppendEntriesRPC(
    term,
    leaderId,
    prevLogIndex,
    prevLogTerm int,
    newEntries []Entry,
    leaderCommit int) (termResult int, success bool) {
    // TODO: Sort newEntries?

    // 1. Reply false if term < currentTerm.
    if term < this.currentTerm {
        return this.currentTerm, false
    }

    // 2. Reply false if log doesn’t contain an entry at prevLogIndex
    //    whose term matches prevLogTerm (see §5.3 of the raft paper).
    if this.log[prevLogIndex].TermNum != prevLogTerm {
        return this.currentTerm, false
    }

    // 3. If an existing entry conflicts with a new one (same index
    //    but different terms), delete the existing entry and all that
    //    follow it (see §5.3 of the raft paper).
    for _, newEntry := range newEntries {
        indexIsInRange := len(this.log) <= newEntry.Index
        if indexIsInRange {
            entryIsUnequal := !cmp.Equal(this.log[newEntry.Index], newEntry)
            if entryIsUnequal {
                this.log = this.log[:newEntry.Index] // todo: check to ensure this works.
            }
        }

    }

    // 4. Append any new entries not already in the log
    this.log = append(this.log, newEntries...)

    // 5. If leaderCommit > commitIndex, set commitIndex =
    //    min(leaderCommit, index of last new entry).
    if leaderCommit > this.commitIndex {
        this.commitIndex = minInt(leaderCommit, lastEntry(newEntries).Index)
    }

    return this.currentTerm, true
}

func (this *Node) RequestVoteRPC(
    term,
    candidateId,
    lastLogIndex,
    lastLogTerm int) (termResult int, voteGranted bool) {

    //1. Reply false if term < currentTerm (see §5.1 of the raft paper)
    if term < this.currentTerm {
        return this.currentTerm, false
    }

    // 2. If votedFor is null or candidateId, and candidate’s log
    //    is at least as up-to-date as receiver’s log (see below),
    //    grant vote (see §5.2 and §5.4 of the raft paper)
    //
    //    If the logs have last entries with different terms,
    //    then the log with the later term is more up-to-date.
    //    If the logs end with the same term, then whichever
    //    log is longer is more up-to-date.
    notYetVoted := this.votedFor == -1
    votedSameBefore := this.votedFor == candidateId
    requesterMoreUpToDate := lastEntry(this.log).TermNum <= term
    if (notYetVoted || votedSameBefore) && requesterMoreUpToDate {
        return this.currentTerm, true
    }
}

// minInt finds Min of ints.
func minInt(a, b int) int {
    if a < b {
        return a
    }
    return b
}

// lastEntry find last Entry in slice of Entries.
func lastEntry(ents []Entry) Entry {
    return ents[len(ents)-1]
}
