# `kubectl pods-on`

A kubectl plugin to query list of pods running on a Node (by name or Node label
selector).

If you ever found yourself finding a list of Pods on a particular Node or a
set of Nodes, you'll find this plugin useful.

### Features

- Query multiple Node names at the same time.
- Specify Node selectors (instead of Node names) to query
- Supports `-o/--output=json|yaml|wide|jsonpath|go-template|...` formats (just
  like `kubectl`)
- Performance optimizations like parallel queries.
- Runs fast on large clusters, as it employs different query strategies based on
  the cluster size.

### Examples

- List all pods running on a node (or more nodes):

  ```sh
  kubectl pods-on <node-name> [<node-name>...]
  ```

- List all pods running on nodes with a specific label:

  ```sh
  kubectl pods-on pool=general
  ```

- List all pods running on nodes that match a particular selector:

  ```sh
  kubectl pods-on "topology.kubernetes.io/zone in (us-west-1a, us-west-1b)"
  ```

- A combination of both syntaxes (the results of each selector will be OR'ed):

  ```sh
  kubectl pods-on \
    "tier in (db, cache)" \
    "foo=bar"\
    node1.example.com
  ```

### Installation

Until it is packaged as a kubectl plugin via Krew, you can install it manually:

1. ```sh
   go install github.com/ahmetb/kubectl-pods_on@latest
   ```

2. Add `$HOME/go/bin` to your `PATH`.

3. Run `kubectl pods-on`!

### License

Distributed as-is under Apache 2.0. See [LICENSE](./LICENSE).
