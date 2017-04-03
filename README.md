# BestBoughtServer (UBC BizHacks 2017)

#### Route:
/product/{BestBuyProductId}

#### Description:
- Given a product id, the api retrieves the list of comments from the best buy product page using their open api
`http://www.bestbuy.ca/api/v2/json/reviews/{BestBuyProductId}?page=1&pagesize=50&source=us"`
- The comments are then sent to Microsoft Cognitive Services Text analyzer to determine each comments semantic score and key phrases
- Each comment is scored through our algorithm to return a ranked list of comments based on "usefulness" and returned to the client

#### Tech used:
- Microsoft Azure
- Golang 1.8
- Microsoft Cognitive Services
