package main

import (
    "github.com/google/go-cmp/cmp"
    "time"
    "fmt"
    "sync"
    "math/rand"
)

// The values of the various timeouts used in the Raft Algorithm.
const (
    FollowerPeriod  = 3 * time.Second
    CandidatePeriod = 3 * time.Second
    LeaderPeriod    = 3 * time.Second
    HeartBeatPeriod = 2 * time.Second
)

type NodeType int

// The server roles given in the Raft paper.
const (
    Leader    NodeType = iota
    Follower
    Candidate
)

// Node stores the state for (virtual) "servers" in this
// Raft implemenation.
type Node struct {
    // IMPLEMENTATION SPECIFIC STATE:
    // The following values are specific to this implementation
    // and are not a part of the raft consensus algorithm.

    // Node ID.
    id int

    // Role of the node.
    nodeType NodeType

    // State Machine
    stateMachine func(string)

    // List of other nodes participating in the protocol.
    peers []*Node

    //countDownTimer *timoutTicker // Externally declared so it can be reset.
    heartBeatInput chan string // Externally declared so it can be accessed.

    // RAFT SPECIFIC STATE:
    // The following values are from the states
    // described in the raft paper:

    // PERSISTENT STATE:

    // Latest term server has seen (initialized to 0
    // on first boot, increases monotonically).
    currentTerm int

    // CandidateId that received vote in current
    // term (or null if none).
    votedFor      int
    votedForMutex *sync.Mutex

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

// NewNode initializes a new Node. Nodes act as the virtual
// "servers" in this Raft implementation.
func NewNode(id int, stateMachine func(string)) (this *Node) {

    this = new(Node)

    this.id = id
    this.heartBeatInput = make(chan string, 1000)
    this.stateMachine = stateMachine
    this.votedForMutex = &sync.Mutex{}

    // Initialize (non-leader)State described in the Raft paper.
    this.currentTerm = 0
    this.votedFor = -1
    this.log = make([]Entry, 1) // TODO: Initialize to 1?
    this.commitIndex = 0
    this.lastApplied = 0

    this.BecomeFollower()
    return this
}

func (this *Node) AddNodeToCluster(that *Node) ([]*Node) {
    // Distribute knowledge to peers.
    // In a real-world scenario, this would be handled by a
    // configuration manager, such as Zookeeper.
    this.peers = append(this.peers, that)
    for _, node := range this.peers {
        node.peers = this.peers
    }
    return this.peers
}

func CreateCluster(numNodes int, stateMachine func(string)) ([]*Node) {
    nodes := make([]*Node, numNodes)
    for i := range nodes {
        nodes[i] = NewNode(i, stateMachine)
    }
    for i := range nodes {
        nodes[i].peers = nodes
    }
    return nodes
}

// BecomeLeader implements the logic to convert a node
// to be in the "Leader" state.
func (this *Node) BecomeLeader() {
    go func() {
        fmt.Println("Node: ", this.id, " became leader.")

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
            this.matchIndex[i] = 0 // TODO: ensure this is correct, will need to iteratively increment values to match followers later
        }
        // TODO: Leader needs a goroutine with a quit chanel
        // goroutine routinly broadcasts hearbeats.
        // each hearbeat has info from broadcast channel.

        heartBeatTimer := newTimoutTicker(HeartBeatPeriod)
        for {
            select {
            case <-heartBeatTimer.ticker.C:
                for _, node := range this.peers {
                    node.heartBeatInput <- "Message"
                }
            default:
            }
        }

    }()
}

// BecomeFollower implements the logic to convert a node
// to be in the "Follower" state.
func (this *Node) BecomeFollower() {
    go func() {
        fmt.Println("Node: ", this.id, " became follower.")
        this.nodeType = Follower

        // Remove leader attributes.
        this.nextIndex = nil
        this.matchIndex = nil

        // Set timer.
        countDownTimer := newTimoutTicker(followerRandPeriod())
        for {
            select {
            case val := <-this.heartBeatInput:
                fmt.Println("Node: ", this.id, " received a heartbeat: ", val)
                countDownTimer.reset()

                // TODO: reset votedFor if this term is greater in heartbeat.
                // TODO apply update in `val` to state machine.
            case <-countDownTimer.ticker.C:
                fmt.Println("Node: ", this.id, " timed out.")
                this.BecomeCandidate()
                return
            default:
            }
        }
    }()
}

// BecomeCandidate implements the logic to convert a node
// to be in the "Candidate" state.
func (this *Node) BecomeCandidate() {
    go func() {
        fmt.Println("Node: ", this.id, " became Candidate.")

        // To begin an election, a follower increments its current
        // term and transitions to candidate state (see §5.2 of the
        // raft paper).
        this.currentTerm++
        this.nodeType = Candidate

        // Remove leader attributes.
        this.nextIndex = nil
        this.matchIndex = nil

        // TODO: No timeout logic yet.
        // TODO: problem, how to cancel after timeout. https://gobyexample.com/timeouts
        // TODO: RequestVoteRPC should be called in parallel according to paper.
        yes_votes := 0
        no_votes := 0
        for _, node := range this.peers {
            termResult, voteGranted := node.RequestVoteRPC(
                this.currentTerm,
                this.id,
                this.commitIndex,
                this.currentTerm) // TODO: ensure these params are correct.

            // Cancel election if another node is discovered
            // to have a higher term.
            if termResult > this.currentTerm {
                this.BecomeFollower()
                return
            }

            // Record vote.
            if voteGranted {
                yes_votes++
            } else {
                no_votes++
            }

        }

        // Act on results of election.
        if (yes_votes ) > no_votes {
            this.BecomeLeader()
        } else {
            this.BecomeFollower()
        }
    }()
}

// AppendEntriesRPC implements the logic geven in the
// "AppendEntries RPC" section on pg4 of the Raft paper.
func (this *Node) AppendEntriesRPC(
    term,
    leaderId,
    prevLogIndex,
    prevLogTerm int,
    newEntries []Entry,
    leaderCommit int) (termResult int, success bool) {
    // TODO: Sort newEntries?

    // Abdicate leadership if requester has higher term.
    this.checkToAbdicateLeadership(term) //TODO needs updating.

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

    // 4. Append any new entries not already in the log.
    this.log = append(this.log, newEntries...)

    // 5. If leaderCommit > commitIndex, set commitIndex =
    //    min(leaderCommit, index of last new entry).
    if leaderCommit > this.commitIndex {
        this.commitIndex = minInt(leaderCommit, lastEntry(newEntries).Index)
    }

    return this.currentTerm, true
}

// RequestVoteRPC implements the logic geven in the
// "RequestVote RPC" section on pg4 of the Raft paper.
func (this *Node) RequestVoteRPC(
    term,
    candidateId,
    lastLogIndex,
    lastLogTerm int) (termResult int, voteGranted bool) {

    // Abdicate leadership if requester has a higher term.
    this.checkToAbdicateLeadership(term)

    // 1. Reply false if term < currentTerm (see §5.1 of the raft paper).
    if term < this.currentTerm {
        return this.currentTerm, false
    }

    // 2. If votedFor is null or candidateId, and candidate’s log
    //    is at least as up-to-date as receiver’s log (see below),
    //    grant vote (see §5.2 and §5.4 of the raft paper).
    //
    //    If the logs have last entries with different terms,
    //    then the log with the later term is more up-to-date.
    //    If the logs end with the same term, then whichever
    //    log is longer is more up-to-date.
    this.votedForMutex.Lock()
    notYetVoted := this.votedFor == -1
    votedSameBefore := this.votedFor == candidateId
    requesterMoreUpToDate := lastEntry(this.log).TermNum <= term
    if (notYetVoted || votedSameBefore) && requesterMoreUpToDate {
        this.votedFor = candidateId
        this.votedForMutex.Unlock()
        return this.currentTerm, true
    } else {
        this.votedForMutex.Unlock()
    }

    return this.currentTerm, false
}

func (this *Node) checkToAbdicateLeadership(term int) {
    // Ensure the following property:
    // If RPC request or response contains
    // term T > currentTerm: set currentTerm = T,
    // convert to follower (see §5.1 of the raft
    // paper).

    if term > this.currentTerm {
        this.currentTerm = term
        this.nodeType = Follower
    }
}

// timoutTicker stores the state for the timoutTicker logic
// for heartbeats and candidacy timeouts.
type timoutTicker struct {
    period time.Duration
    ticker time.Ticker
}

// reset the timoutTicker time, (i.e. when a heartbeat msg is
// received).
func (t *timoutTicker) reset() {
    t.ticker = *time.NewTicker(t.period)
}

// newTimoutTicker instantiates a new ticker struct.
func newTimoutTicker(period time.Duration) *timoutTicker {
    ticker := new(timoutTicker)
    ticker.period = period
    ticker.ticker = *time.NewTicker(period)
    return ticker
}

func followerRandPeriod() time.Duration {
    return FollowerPeriod + time.Duration(rand.Intn(10))*time.Millisecond
}

// ServerEventLoop implements the "Rules for Servers" on pg4
// of the Raft paper. It is the primary control structure (or
// "main" function for each node.
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
                // state machine (see §5.3 of the raft paper).
                if this.commitIndex > this.lastApplied {
                    this.lastApplied += 1
                    this.stateMachine(this.log[this.lastApplied].Command)
                }
                // If RPC request or response contains term
                // T > currentTerm: set currentTerm = T,
                // convert to follower (§5.1).

                // TODO: Not finished

            }
        }
        // TODO: might need a waitgroup
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

//// TEST CODE /////////////////////////////
/*
Notes:
    - check to see if thread blocks when putting item on full chan.
 */
func stateMachineFactory() func(string) {
    return func(text string) {
        print(text)
    }
}
func main() {
    CreateCluster(3, stateMachineFactory())

    var wg sync.WaitGroup
    wg.Add(1)
    wg.Wait()

}
