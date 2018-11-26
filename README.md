# The Raft Consensus Algorithm in GoLang 

**Note: This codebase is still a work in progress.**


## Introduction to Raft
The [Consensus Problem](https://en.wikipedia.org/wiki/Consensus_(computer_science)): 
can be described as: "The problem of achieving overall system 
reliability in the presence of a number of faulty processes 
in a distributed system".

Raft is a Consensus Algorithm, meaning that it is a solution 
to the Consensus Problem 


The following resources give a good introduction to the algorithm
* [The official webside](https://raft.github.io/)
* [A guided visualization](http://thesecretlivesofdata.com/raft/)
* [The Raft paper](https://raft.github.io/raft.pdf)

## Implementation
This implementation of the Raft Consensus Algorithm is meant to 
be pedagogical. Most of the other solutions implemented in 
golang that can be found online are fully fledged production 
code.  Production code is an un-ideal medium for learning,
since configuration overhead adds a great deal of complexity 
(i.e. setting up ports, docker instances, TLS, networking, 
etc.).

With this in mind, this implementation seeks to do away with networking
by running virtual server nodes as goroutines. This reduces
the configuration footprint, and makes it easier for aspiring
Rafters to play with the code and make sense of the system.

Please note that this Repo is not yet finished, but will be
available by mid Dec 2018.

<embed src="https://github.com/ongardie/raftscope/blob/master/index.html">
<iframe src="https://github.com/ongardie/raftscope/blob/master/index.html" style="border: 0; width: 800px; height: 580px; margin-bottom: 20px"></iframe>
