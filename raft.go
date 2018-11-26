package main

import (
    "github.com/google/go-cmp/cmp"
    "time"
    "fmt"
)

const (
    FollowerPeriod  = 3 * time.Second
    CandidatePeriod = 3 * time.Second
    LeaderPeriod    = 3 * time.Second
    HeartBeatPeriod = 2 * time.Second
)

type NodeType int

const (
    Leader    NodeType = iota
    Follower
    Candidate
)

type Node struct {
    // IMPLEMENTATION SPECIFIC STATE:
    // The following values are specific to this implementation
    // and are not a part of the raft consensus algorithm.

    // Node ID
    id int

    // Role of the node.
    nodeType NodeType

    // State Machine
    stateMachine func(string)

    // List of other nodes participating in the protocol.
    peers []*Node

    timeout                 *ticker     // Externally declared so it can be reset
    heartBeatInput          chan string // Externally declared so it can be accessed
    killCurrentListenerChan chan bool   // Externally declared so it can be accessed

    // RAFT SPECIFIC STATE:
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

type Entry struct {
    Command string
    Index   int
    TermNum int
}

func NewNode(id int, peers []*Node, stateMachine func(string)) (this *Node) {
    this = new(Node)

    this.id = id
    this.BecomeFollower()
    this.heartBeatInput = make(chan string)
    this.stateMachine = stateMachine

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

func (this *Node) BecomeLeader() {
    this.nodeType = Leader

    // Initialize all nextIndex values to the index value just
    // after the last index in the log. (The log starts at 1.)
    this.nextIndex = make([]int, len(this.peers))
    for i := range this.nextIndex {
        this.nextIndex[i] = len(this.log) + 1
    }

    // For each server, index of highest log entry
    // known to be replicated on server (initialized
    // to 0, increases monotonically).
    this.matchIndex = make([]int, len(this.peers))
    for i := range this.matchIndex {
        this.matchIndex[i] = 0 //TODO: ensure this is correct, will need to iteratively increment values to match followers later
    }

    // Leaders don't timeout, so remove listener closure.
    this.timeout = nil
    this.killCurrentListenerChan <- true
}

func (this *Node) BecomeFollower() {
    this.nodeType = Follower

    // Remove leader attributes.
    this.nextIndex = nil
    this.matchIndex = nil

    // Reset listener closure.
    this.timeout = newTicker(CandidatePeriod)
    this.killCurrentListenerChan <- true
    go this.listener(this.heartBeatInput, this.BecomeCandidate)
}

func (this *Node) BecomeCandidate() {
    this.nodeType = Candidate

    // Remove leader attributes.
    this.nextIndex = nil
    this.matchIndex = nil

    // Reset listener closure.
    this.timeout = newTicker(CandidatePeriod)
    this.killCurrentListenerChan <- true
    go this.listener(this.heartBeatInput, this.BecomeCandidate)

    //TODO: implement logic for requesting voting

}

func (this *Node) AppendEntriesRPC(
    term,
    leaderId,
    prevLogIndex,
    prevLogTerm int,
    newEntries []Entry,
    leaderCommit int) (termResult int, success bool) {
    // TODO: Sort newEntries?

    // Abdicate leadership if requester has higher term.
    this.testToAbdicateLeadership(term)

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

    // Abdicate leadership if requester has a higher term.
    this.testToAbdicateLeadership(term)

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

    return this.currentTerm, false
}

func (this *Node) testToAbdicateLeadership(term int) {
    // Ensure the following property:
    // If RPC request or response contains
    // term T > currentTerm: set currentTerm = T,
    // convert to follower (see §5.1 of the raft
    // paper)

    if term > this.currentTerm {
        this.currentTerm = term
        this.nodeType = Follower
    }
}

type ticker struct {
    period time.Duration
    ticker time.Ticker
}

func (t *ticker) reset() {
    t.ticker = *time.NewTicker(t.period)
}

func newTicker(period time.Duration) *ticker {
    return &ticker{period, *time.NewTicker(period)}
}

// listener used to act on heartbeat signals.
// Will call callback on timeout.
func (this *Node) listener(heartBeat chan string, callback func()) {
    for {
        select {
        case msg := <-heartBeat:
            this.timeout.reset()
            fmt.Println("received: ", msg, " from hearbeat") //TODO replace with action callback
        case <-this.timeout.ticker.C:
            // call to change state
            callback()
        case <-this.killCurrentListenerChan:
            return
        }
    }
}

func (this *Node) ServerEventLoop(quit chan int) {

    // Note: Node could die any time, instead of dying in a
    // well-behaved state like what is is shown when quit is
    // called.
    go func() {
        for {
            select {
            case <-quit:
                return
            default:
                // If commitIndex > lastApplied: increment
                // lastApplied, apply log[lastApplied] to
                // state machine (see §5.3 of the raft paper)
                if this.commitIndex > this.lastApplied {
                    this.lastApplied += 1
                    this.stateMachine(this.log[this.lastApplied].Command)
                }

            }
        }
        //TODO: might need a waitgroup
    }()

    // If commitIndex > lastApplied: increment lastApplied, apply
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

/*
check to see if thread blocks when putting item on full chan
