TODO:


## Config changess

- protocol version will show if config functions are available
- config can autogenerate devices, and use the registry to get deviceconfig
- reduce the need to import libconfig in the main function



## Ideas / Brainstorm

- Think about the use of feedback delayed / feedback normal...

- Autodelay things ? or at least warnings ? make warnings that can be disabled with a tag or env ?
How a bout a little env helper lib that can give you all set tags on a thing ?

- RegisterDefaultUnavaiable... to reduce state load...

- Dynamic dimension elements... could be simpler to pull off than I thought, 
could have a new value type called DeleteDimension to update, 
to "Add" simply broadcast the values
