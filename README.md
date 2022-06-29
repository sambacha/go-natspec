# [go-natspec](#)

## Requirements

- Pygments installed.   
- Pygments solidity lexer available  

### macOS

```shell
brew install pygments
```

[https://gitlab.com/veox/pygments-lexer-solidity/-/blob/master/pygments_lexer_solidity/lexer.py](https://gitlab.com/veox/pygments-lexer-solidity/-/blob/master/pygments_lexer_solidity/lexer.py)

## Usage

```shell
wget https://github.com/sambacha/go-natspec/releases/download/v0.0.1/dappspec
chmod +x dappspec
./dappspec test.sol
> dappspec:  test.sol  ->  docs/test.html
serve docs/
--> http://localhost:3000/test
```

Download binary.    
use binary on Solidity file.    
documents generated to docs/ dir (make sure this exists).    
