# Asynchronous execution

Some actions must be executed in a asynchronous manner. To simplify this difficult implementation challenge, we provide two async primitives within this package:

- task provides a workflow like interface
- queue provides a fifo queue

Currently these are used in three scenarios, one for machine bmc command execution, one for machine allocation (not yet implemented) and for transactions which must be spanned across multiple entities and backends.

## Machine BMC Command execution

The apiv2 has a `apiv2.BMCCommand` rpc call, which once executed will fire the specified command against the bmc of the machine.

The workflow of this command execution is illustrated below:

![machine-bmc-command](machine-bmc-command.drawio.svg)

## Machine Allocation

TODO describe and draw a flow diagram

## Transactions

TODO describe and draw a flow diagram